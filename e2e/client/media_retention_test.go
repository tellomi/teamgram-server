package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMediaRetention runs the retention sweeper inside the teamgram
// container against media whose upload time is pushed 100 days back, then
// checks what upload.getFile still serves. It needs docker compose access,
// like TestReconnectAfterGatewayRestart.
//
// Users: alice and erin are free, bob is an active subscriber, carol's
// subscription lapsed 10 days ago (inside the 30-day grace), dave's lapsed
// 60 days ago.
func TestMediaRetention(t *testing.T) {
	cfg := requireE2E(t)
	if os.Getenv("TELLOMI_E2E_RESTART") == "" {
		t.Skip("set TELLOMI_E2E_RESTART=1 (docker compose access) to run the retention sweeper")
	}
	a, alice := loginNamed(t, cfg, ProfileIOS, "ret_alice", "RetAlice")
	_, bob := loginNamed(t, cfg, ProfileIOS, "ret_bob", "RetBob")
	_, carol := loginNamed(t, cfg, ProfileIOS, "ret_carol", "RetCarol")
	d, dave := loginNamed(t, cfg, ProfileAndroid, "ret_dave", "RetDave")
	_, erin := loginNamed(t, cfg, ProfileIOS, "ret_erin", "RetErin")

	peer := func(c *Client, name, first string) *mtproto.InputPeer {
		t.Helper()
		ph, err := cfg.Phone(name)
		if err != nil {
			t.Fatal(err)
		}
		cx, cancel := ctx()
		defer cancel()
		id, hash, err := c.ImportPhone(cx, ph.Number, first)
		if err != nil {
			t.Fatal(err)
		}
		return inputPeerUser(id, hash)
	}
	aToDave := peer(a, "ret_dave", "RetDave")
	aToBob := peer(a, "ret_bob", "RetBob")
	daveToCarol := peer(d, "ret_carol", "RetCarol")
	daveToErin := peer(d, "ret_erin", "RetErin")
	daveToAlice := peer(d, "ret_alice", "RetAlice")

	setPremium := func() {
		now := time.Now().Unix()
		mustSQL(t, fmt.Sprintf(`UPDATE users SET premium = 0, premium_expire_date = 0 WHERE id IN (%d, %d);
UPDATE users SET premium = 1, premium_expire_date = %d WHERE id = %d;
UPDATE users SET premium = 0, premium_expire_date = %d WHERE id = %d;
UPDATE users SET premium = 1, premium_expire_date = %d WHERE id = %d;`,
			alice.ID, erin.ID,
			now+30*86400, bob.ID,
			now-10*86400, carol.ID,
			now-60*86400, dave.ID))
	}
	setPremium()
	// Wait until the dfs/media backends are registered in etcd.
	waitMediaReady(t, a, aToDave)
	t.Cleanup(func() {
		_, _ = runSQL(fmt.Sprintf(`UPDATE users SET premium = 0, premium_expire_date = 0 WHERE id IN (%d, %d, %d, %d, %d)`,
			alice.ID, bob.ID, carol.ID, dave.ID, erin.ID))
	})

	// Free private chat: expires.
	freeDoc, _ := sendDoc(t, a, aToDave, "video/x-matroska", nil)
	// Free user with a subscriber: kept.
	paidDoc, _ := sendDoc(t, a, aToBob, "application/pdf", nil)
	// Basic group alice+dave+bob (mixed free/paid): kept.
	chatID := func() int64 {
		cx, cancel := ctx()
		defer cancel()
		id, err := a.CreateChat(cx, fmt.Sprintf("ret-%d", time.Now().UnixNano()),
			inputUser(aToDave.GetUserId(), aToDave.GetAccessHash()), inputUser(aToBob.GetUserId(), aToBob.GetAccessHash()))
		if err != nil {
			t.Fatal(err)
		}
		return id
	}()
	groupDoc, _ := sendDoc(t, a, inputPeerChat(chatID), "audio/ogg", []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration: 2}).To_DocumentAttribute(),
	})
	// Forwarded into a chat with carol (grace): kept, because carol still has it.
	graceDoc, _ := sendDoc(t, a, aToDave, "video/x-matroska", nil)
	// Forwarded between free users: the original upload date counts.
	fwdDoc, _ := sendDoc(t, a, aToDave, "application/zip", nil)
	// GIFs are exempt. A real GIF upload is converted to mp4 with
	// documentAttributeAnimated (covered by the unit tests); a plain
	// image/gif document skips ffmpeg here.
	gifDoc, _ := sendDoc(t, a, aToDave, "image/gif", nil)
	// Young media in a free chat: kept.
	youngDoc, _ := sendDoc(t, a, aToDave, "video/x-matroska", nil)
	// Photo in a free chat: full sizes expire, the small thumbnail stays.
	photo := sendPhoto(t, a, aToDave)

	// dave forwards his copies. The forwarded messages share the document id.
	forward(t, d, daveToAlice, graceDoc.GetId(), daveToCarol)
	forward(t, d, daveToAlice, fwdDoc.GetId(), daveToErin)

	old := time.Now().Add(-100 * 24 * time.Hour).Unix()
	ids := []int64{freeDoc.GetId(), paidDoc.GetId(), groupDoc.GetId(), graceDoc.GetId(), fwdDoc.GetId(), gifDoc.GetId()}
	mustSQL(t, fmt.Sprintf(`UPDATE documents SET date2 = %d WHERE document_id IN (%s);
UPDATE photos SET date2 = %d WHERE photo_id = %d;`, old, joinIDs(ids), old, photo.GetId()))

	// Dry run: reports, changes nothing.
	out := sweep(t, true)
	for _, id := range []int64{freeDoc.GetId(), fwdDoc.GetId(), photo.GetId()} {
		if !strings.Contains(out, fmt.Sprintf(" %d ", id)) {
			t.Errorf("dry run does not list %d:\n%s", id, out)
		}
	}
	for _, id := range []int64{paidDoc.GetId(), groupDoc.GetId(), graceDoc.GetId(), gifDoc.GetId(), youngDoc.GetId()} {
		if strings.Contains(out, fmt.Sprintf(" %d ", id)) {
			t.Errorf("dry run lists kept media %d:\n%s", id, out)
		}
	}
	if n := strings.TrimSpace(mustSQL(t, fmt.Sprintf(`SELECT COUNT(*) FROM documents WHERE expired_at <> 0 AND document_id IN (%s)`, joinIDs(ids)))); n != "0" {
		t.Fatalf("dry run marked %s documents expired", n)
	}

	sweep(t, false)

	expectGone := func(name string, loc *mtproto.InputFileLocation) {
		t.Helper()
		cx, cancel := ctx()
		defer cancel()
		_, err := a.Invoke(cx, &mtproto.TLUploadGetFile{Constructor: mtproto.CRC32_upload_getFile_be5335be, Location: loc, Limit: 64 * 1024})
		rpc, ok := asRPCError(err)
		if !ok || rpc.Message != "FILE_ID_INVALID" {
			t.Errorf("%s: getFile err %v, want FILE_ID_INVALID", name, err)
		}
	}
	expectBytes := func(name string, loc *mtproto.InputFileLocation) {
		t.Helper()
		cx, cancel := ctx()
		defer cancel()
		res, err := a.Invoke(cx, &mtproto.TLUploadGetFile{Constructor: mtproto.CRC32_upload_getFile_be5335be, Location: loc, Limit: 64 * 1024})
		if err != nil || len(bytesOf(res)) == 0 {
			t.Errorf("%s: getFile err %v bytes %d", name, err, len(bytesOf(res)))
		}
	}
	expectGone("free private chat", docLocation(freeDoc))
	expectGone("forwarded between free users", docLocation(fwdDoc))
	expectGone("photo full size", photoLocation(photo, "x"))
	expectGone("photo medium size", photoLocation(photo, "m"))
	expectBytes("photo small thumbnail", photoLocation(photo, "s"))
	expectBytes("chat with a subscriber", docLocation(paidDoc))
	expectBytes("mixed free/paid group", docLocation(groupDoc))
	expectBytes("forwarded to a user in grace", docLocation(graceDoc))
	expectBytes("gif", docLocation(gifDoc))
	expectBytes("young", docLocation(youngDoc))

	// Text survives: the message that carried the expired file is still there.
	if n := strings.TrimSpace(mustSQL(t, fmt.Sprintf(`SELECT COUNT(*) FROM messages WHERE user_id = %d AND deleted = 0 AND JSON_VALUE(message_data, '$.media.document.id' RETURNING SIGNED NULL ON EMPTY NULL ON ERROR) IN (%d)`, dave.ID, freeDoc.GetId()))); n == "0" {
		t.Error("message with expired media was removed")
	}
}

func joinIDs(ids []int64) string {
	s := make([]string, len(ids))
	for i, id := range ids {
		s[i] = fmt.Sprint(id)
	}
	return strings.Join(s, ",")
}

func docLocation(doc *mtproto.Document) *mtproto.InputFileLocation {
	return mtproto.MakeTLInputDocumentFileLocation(&mtproto.InputFileLocation{
		Id: doc.GetId(), AccessHash: doc.GetAccessHash(), FileReference: doc.GetFileReference(),
	}).To_InputFileLocation()
}

func photoLocation(p *mtproto.Photo, size string) *mtproto.InputFileLocation {
	return mtproto.MakeTLInputPhotoFileLocation(&mtproto.InputFileLocation{
		Id: p.GetId(), AccessHash: p.GetAccessHash(), FileReference: p.GetFileReference(), ThumbSize: size,
	}).To_InputFileLocation()
}

func uploadBytes(t *testing.T, c *Client, b []byte) int64 {
	t.Helper()
	fileID := randomID()
	invokeOK(t, c, &mtproto.TLUploadSaveFilePart{Constructor: mtproto.CRC32_upload_saveFilePart, FileId: fileID, FilePart: 0, Bytes: b})
	return fileID
}

func sendDoc(t *testing.T, c *Client, to *mtproto.InputPeer, mime string, attrs []*mtproto.DocumentAttribute) (*mtproto.Document, int32) {
	t.Helper()
	payload := make([]byte, 4096)
	_, _ = rand.Read(payload)
	fileID := uploadBytes(t, c, payload)
	attrs = append(attrs, mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: "retention.bin"}).To_DocumentAttribute())
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:       mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "retention.bin"}).To_InputFile(),
			MimeType:   mime,
			Attributes: attrs,
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if doc := m.GetMedia().GetDocument(); doc != nil {
			return doc, m.GetId()
		}
	}
	t.Fatalf("sendMedia %s returned no document (%T)", mime, res)
	return nil, 0
}

func sendPhoto(t *testing.T, c *Client, to *mtproto.InputPeer) *mtproto.Photo {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1000, 700))
	for y := 0; y < 700; y++ {
		for x := 0; x < 1000; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	if buf.Len() > 512*1024 {
		t.Fatalf("test jpeg too large: %d", buf.Len())
	}
	fileID := uploadBytes(t, c, buf.Bytes())
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "retention.jpg"}).To_InputFile(),
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if p := m.GetMedia().GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 {
			return p
		}
	}
	t.Fatalf("sendMedia photo returned no photo (%T)", res)
	return nil
}

// forward finds c's own copy of the message carrying docID in its chat
// with from (private chats number messages per box) and forwards it to to.
func forward(t *testing.T, c *Client, from *mtproto.InputPeer, docID int64, to *mtproto.InputPeer) {
	t.Helper()
	hist := invokeOK(t, c, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        from,
		Limit:       50,
	})
	var id int32
	for _, m := range allMessages(hist) {
		if m.GetMedia().GetDocument().GetId() == docID {
			id = m.GetId()
		}
	}
	if id == 0 {
		t.Fatalf("document %d not in history", docID)
	}
	res := invokeOK(t, c, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    from,
		Id:          []int32{id},
		RandomId:    []int64{randomID()},
		ToPeer:      to,
	})
	for _, m := range allMessages(res) {
		if m.GetMedia().GetDocument().GetId() == docID {
			return
		}
	}
	t.Fatalf("forward of document %d returned %T without it", docID, res)
}

func composeBase() []string {
	files := os.Getenv("TELLOMI_E2E_COMPOSE_FILES")
	if files == "" {
		files = "docker-compose-env.yaml,docker-compose.yaml"
	}
	args := []string{"compose"}
	if project := os.Getenv("COMPOSE_PROJECT_NAME"); project != "" {
		args = append(args, "-p", project)
	}
	for _, f := range strings.Split(files, ",") {
		args = append(args, "-f", strings.TrimSpace(f))
	}
	return args
}

func composeExec(timeout time.Duration, stdin string, cmdline ...string) (string, error) {
	cx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	args := append(composeBase(), "exec", "-T", "teamgram")
	args = append(args, cmdline...)
	cmd := exec.CommandContext(cx, "docker", args...)
	if dir := os.Getenv("TELLOMI_E2E_ROOT"); dir != "" {
		cmd.Dir = dir
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return string(out), nil
}

func runSQL(q string) (string, error) {
	return composeExec(time.Minute, q, "mysql", "-hmysql", "-uteamgram", "-pteamgram", "-N", "-B", "teamgram")
}

func mustSQL(t *testing.T, q string) string {
	t.Helper()
	out, err := runSQL(q)
	if err != nil {
		t.Fatal(err)
	}
	// Drop the client's password warning.
	var keep []string
	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "mysql: [Warning]") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

func sweep(t *testing.T, dryRun bool) string {
	t.Helper()
	out, err := composeExec(5*time.Minute, "", "/app/bin/mediaretention", "-f", "/app/etc2/media.yaml", fmt.Sprintf("-dry-run=%v", dryRun))
	t.Logf("mediaretention dry_run=%v:\n%s", dryRun, out)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// waitMediaReady retries a small document send while the gRPC backends are
// still re-registering after a container restart ("produced zero addresses").
func waitMediaReady(t *testing.T, c *Client, to *mtproto.InputPeer) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		fileID := randomID()
		cx, cancel := ctx()
		_, err := c.Invoke(cx, &mtproto.TLUploadSaveFilePart{Constructor: mtproto.CRC32_upload_saveFilePart, FileId: fileID, FilePart: 0, Bytes: []byte("ready")})
		if err != nil {
			err = fmt.Errorf("upload.saveFilePart: %w", err)
		} else {
			_, err = c.Invoke(cx, &mtproto.TLMessagesSendMedia{
				Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
				Peer:        to,
				Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
					File:     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "ready.txt"}).To_InputFile(),
					MimeType: "text/plain",
					Attributes: []*mtproto.DocumentAttribute{
						mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: "ready.txt"}).To_DocumentAttribute(),
					},
				}).To_InputMedia(),
				RandomId: randomID(),
			})
			if err != nil {
				err = fmt.Errorf("messages.sendMedia: %w", err)
			}
		}
		cancel()
		if err == nil {
			return
		}
		last = err
		time.Sleep(2 * time.Second)
	}
	diag, _ := composeExec(time.Minute, "", "sh", "-c",
		"ps -eo pid,etime,args | grep /app/bin/ | grep -v grep; for f in /app/logs/media/error.log /app/logs/dfs/error.log /app/logs/msg/error.log /app/logs/bff/error.log; do echo \"== $f\"; tail -n 15 $f 2>&1 | cut -c1-400; done")
	t.Logf("teamgram diagnostics:\n%s", diag)
	t.Fatalf("media backends not ready: %v", last)
}
