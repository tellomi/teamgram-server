package e2e

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestTTLMediaExpiry covers view-once photo, timed video, and view-once voice
// between ttl_a and ttl_b. B opens each one. A is pushed
// updateReadMessagesContents with a date. After expiry both histories show
// the empty media form, getFile fails, forward is refused, and B's second
// session sees the same read update.
func TestTTLMediaExpiry(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	aPhone, err := cfg.Phone("ttl_a")
	if err != nil {
		t.Fatal(err)
	}
	bPhone, err := cfg.Phone("ttl_b")
	if err != nil {
		t.Fatal(err)
	}
	a := dialTTL(t, ctx, cfg, ProfileIOS, aPhone.Number, "TTLAlice")
	b := dialTTL(t, ctx, cfg, ProfileIOS, bPhone.Number, "TTLBob")
	b2 := dialTTL(t, ctx, cfg, ProfileAndroid, bPhone.Number, "TTLBob")
	bobID, bobHash, err := a.ImportPhone(ctx, bPhone.Number, "TTLBob")
	if err != nil {
		t.Fatal(err)
	}
	aliceID, aliceHash, err := b.ImportPhone(ctx, aPhone.Number, "TTLAlice")
	if err != nil {
		t.Fatal(err)
	}
	aToB := inputPeerUser(bobID, bobHash)
	bToA := inputPeerUser(aliceID, aliceHash)

	t.Run("view-once photo", func(t *testing.T) {
		photo, aID := sendTTLPhoto(t, ctx, a, aToB, mtproto.ViewOnceTTL)
		loc := photoLocation(photo, "x")
		bID := waitBox(t, ctx, b, bToA, func(m *mtproto.Message) bool {
			return m.GetMedia().GetPhoto_FLAGPHOTO().GetId() == photo.GetId()
		})
		if !historyMsg(t, ctx, b, bToA, bID).GetMediaUnread() {
			t.Fatal("view-once photo was sent without media_unread")
		}
		openAndCheck(t, ctx, a, b, b2, aToB, bToA, aID, bID, loc)
	})

	t.Run("timed video", func(t *testing.T) {
		doc, aID := sendTTLVideo(t, ctx, a, aToB, 3)
		loc := docLocation(doc)
		bID := waitBox(t, ctx, b, bToA, func(m *mtproto.Message) bool {
			return m.GetMedia().GetDocument().GetId() == doc.GetId()
		})
		if !historyMsg(t, ctx, b, bToA, bID).GetMediaUnread() {
			t.Fatal("timed video was sent without media_unread")
		}
		before := time.Now()
		aPts, aQts, aDate, err := a.State(ctx)
		if err != nil {
			t.Fatal(err)
		}
		readContents(t, ctx, b, bID)
		wait, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := waitUntil(wait, 300*time.Millisecond, func() (bool, error) {
			if date, ok := pushedReadContents(a.Pushed(), aID); ok && freshDate(date, before) {
				return true, nil
			}
			diff, err := a.Difference(ctx, aPts, aQts, aDate)
			if err != nil {
				return false, err
			}
			date, ok := readContentsIn(diff, aID)
			return ok && freshDate(date, before), nil
		}); err != nil {
			t.Fatalf("sender was not pushed updateReadMessagesContents with date: %v; pushed=%s", err, pushedSummary(a.Pushed()))
		}
		if err := waitUntil(wait, 400*time.Millisecond, func() (bool, error) {
			if date, ok := pushedReadContents(b2.Pushed(), bID); ok && freshDate(date, before) {
				return true, nil
			}
			pts, qts, date, err := b2.State(ctx)
			if err != nil {
				return false, err
			}
			from := pts - 5
			if from < 0 {
				from = 0
			}
			diff, err := b2.Difference(ctx, from, qts, date-30)
			if err != nil {
				return false, err
			}
			d, ok := readContentsIn(diff, bID)
			return ok && freshDate(d, before), nil
		}); err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(20 * time.Second)
		for {
			if expiredTTL(historyMsg(t, ctx, a, aToB, aID)) && expiredTTL(historyMsg(t, ctx, b, bToA, bID)) {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("timed video was not expired in both histories")
			}
			time.Sleep(500 * time.Millisecond)
		}
		expectFileGone(t, a, loc)
		expectForwardRefused(t, ctx, b, bToA, aToB, bID)
	})

	t.Run("view-once voice", func(t *testing.T) {
		caption := fmt.Sprintf("ttl-voice-%d", time.Now().UnixNano())
		aID := sendTTLVoice(t, ctx, a, aToB, mtproto.ViewOnceTTL, caption)
		bID := waitBox(t, ctx, b, bToA, func(m *mtproto.Message) bool {
			return m.GetMessage() == caption
		})
		msg := historyMsg(t, ctx, b, bToA, bID)
		loc := docLocation(msg.GetMedia().GetDocument())
		openAndCheck(t, ctx, a, b, b2, aToB, bToA, aID, bID, loc)
	})
}

func dialTTL(t *testing.T, ctx context.Context, cfg Config, profile Profile, number, first string) *Client {
	t.Helper()
	c, err := Dial(ctx, cfg, profile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	if _, err = c.Login(ctx, Phone{Number: number, Code: TestPhoneCode(number)}, first); err != nil {
		t.Fatal(err)
	}
	return c
}

func openAndCheck(t *testing.T, ctx context.Context, a, b, b2 *Client, aToB, bToA *mtproto.InputPeer, aID, bID int32, loc *mtproto.InputFileLocation) {
	t.Helper()
	before := time.Now()
	aPts, aQts, aDate, err := a.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	readContents(t, ctx, b, bID)
	wait, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := waitUntil(wait, 300*time.Millisecond, func() (bool, error) {
		if date, ok := pushedReadContents(a.Pushed(), aID); ok && freshDate(date, before) {
			return true, nil
		}
		diff, err := a.Difference(ctx, aPts, aQts, aDate)
		if err != nil {
			return false, err
		}
		date, ok := readContentsIn(diff, aID)
		return ok && freshDate(date, before), nil
	}); err != nil {
		t.Fatalf("sender was not pushed updateReadMessagesContents with date on %d: %v; pushed=%s", aID, err, pushedSummary(a.Pushed()))
	}
	if err := waitUntil(wait, 300*time.Millisecond, func() (bool, error) {
		date, ok := pushedReadContents(b2.Pushed(), bID)
		if ok && freshDate(date, before) {
			return true, nil
		}
		pts, qts, stDate, err := b2.State(ctx)
		if err != nil {
			return false, err
		}
		from := pts - 5
		if from < 0 {
			from = 0
		}
		diff, err := b2.Difference(ctx, from, qts, stDate-30)
		if err != nil {
			return false, err
		}
		d, ok := readContentsIn(diff, bID)
		return ok && freshDate(d, before), nil
	}); err != nil {
		t.Fatalf("second session did not sync read contents for %d: %v; pushed=%s", bID, err, pushedSummary(b2.Pushed()))
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return expiredTTL(historyMsg(t, ctx, a, aToB, aID)) && expiredTTL(historyMsg(t, ctx, b, bToA, bID)), nil
	}); err != nil {
		t.Fatalf("histories did not show expired media for %d/%d: %v", aID, bID, err)
	}
	expectFileGone(t, b, loc)
	expectForwardRefused(t, ctx, b, bToA, aToB, bID)
}

func readContents(t *testing.T, ctx context.Context, c *Client, id int32) {
	t.Helper()
	if _, err := c.Invoke(ctx, &mtproto.TLMessagesReadMessageContents{
		Constructor: mtproto.CRC32_messages_readMessageContents,
		Id:          []int32{id},
	}); err != nil {
		t.Fatal(err)
	}
}

func freshDate(date int32, before time.Time) bool {
	if date == 0 {
		return false
	}
	opened := time.Unix(int64(date), 0)
	return !opened.Before(before.Add(-2*time.Minute)) && opened.Before(time.Now().Add(2*time.Minute))
}

func expiredTTL(m *mtproto.Message) bool {
	if m == nil || mtproto.TTLSeconds(m) == 0 {
		return false
	}
	media := m.GetMedia()
	switch media.GetPredicateName() {
	case mtproto.Predicate_messageMediaPhoto:
		p := media.GetPhoto_FLAGPHOTO()
		return p == nil || p.GetId() == 0 || p.GetPredicateName() == mtproto.Predicate_photoEmpty
	case mtproto.Predicate_messageMediaDocument:
		d := media.GetDocument()
		return d == nil || d.GetId() == 0 || d.GetPredicateName() == mtproto.Predicate_documentEmpty
	default:
		return false
	}
}

func historyMsg(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, id int32) *mtproto.Message {
	t.Helper()
	msgs, err := c.History(ctx, peer, 40)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range msgs {
		if m.GetId() == id {
			return m
		}
	}
	return nil
}

func waitBox(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, match func(*mtproto.Message) bool) int32 {
	t.Helper()
	var id int32
	err := waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := c.History(ctx, peer, 40)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			if match(m) {
				id = m.GetId()
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil || id == 0 {
		t.Fatalf("message never arrived: %v", err)
	}
	return id
}

func expectForwardRefused(t *testing.T, ctx context.Context, c *Client, from, to *mtproto.InputPeer, id int32) {
	t.Helper()
	_, err := c.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    from,
		Id:          []int32{id},
		RandomId:    []int64{randomID()},
		ToPeer:      to,
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "CHAT_FORWARDS_RESTRICTED" {
		t.Fatalf("forward err %v, want CHAT_FORWARDS_RESTRICTED", err)
	}
}

func pushedSummary(objs []mtproto.TLObject) string {
	var b strings.Builder
	for _, obj := range objs {
		for _, u := range updatesOf(obj) {
			fmt.Fprintf(&b, "%s%v ", u.GetPredicateName(), u.GetMessages())
		}
	}
	if b.Len() == 0 {
		return "none"
	}
	return b.String()
}

func pushedReadContents(objs []mtproto.TLObject, msgID int32) (int32, bool) {
	for _, obj := range objs {
		if date, ok := readContentsIn(obj, msgID); ok {
			return date, true
		}
	}
	return 0, false
}

func readContentsIn(obj mtproto.TLObject, msgID int32) (int32, bool) {
	ups := updatesOf(obj)
	for _, u := range ups {
		if u.GetPredicateName() != mtproto.Predicate_updateReadMessagesContents {
			continue
		}
		for _, id := range u.GetMessages() {
			if id == msgID {
				return u.GetDate_FLAGINT32().GetValue(), true
			}
		}
	}
	return 0, false
}

func sendTTLPhoto(t *testing.T, ctx context.Context, c *Client, to *mtproto.InputPeer, ttl int32) (*mtproto.Photo, int32) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 5), 40, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatal(err)
	}
	fileID := uploadBytes(t, c, buf.Bytes())
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File:       mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "once.jpg"}).To_InputFile(),
			TtlSeconds: &wrapperspb.Int32Value{Value: ttl},
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range allMessages(res) {
		if p := m.GetMedia().GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 {
			return p, m.GetId()
		}
	}
	t.Fatalf("sendMedia photo returned %T", res)
	return nil, 0
}

func sendTTLVideo(t *testing.T, ctx context.Context, c *Client, to *mtproto.InputPeer, ttl int32) (*mtproto.Document, int32) {
	t.Helper()
	payload := bytes.Repeat([]byte("ttl-video"), 64)
	fileID := uploadBytes(t, c, payload)
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:       mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "clip.mkv"}).To_InputFile(),
			MimeType:   "video/x-matroska",
			TtlSeconds: &wrapperspb.Int32Value{Value: ttl},
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: "clip.mkv"}).To_DocumentAttribute(),
				mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{Duration: 1, W: 160, H: 120}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range allMessages(res) {
		if d := m.GetMedia().GetDocument(); d != nil && d.GetId() != 0 {
			return d, m.GetId()
		}
	}
	t.Fatalf("sendMedia video returned %T", res)
	return nil, 0
}

func sendTTLVoice(t *testing.T, ctx context.Context, c *Client, to *mtproto.InputPeer, ttl int32, caption string) int32 {
	t.Helper()
	fileID := uploadBytes(t, c, voiceOgg)
	res, err := c.Invoke(ctx, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:       mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "voice.ogg"}).To_InputFile(),
			MimeType:   "audio/ogg",
			TtlSeconds: &wrapperspb.Int32Value{Value: ttl},
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration_INT32: 1}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
		Message:  caption,
		RandomId: randomID(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := messageID(res)
	if id == 0 {
		t.Fatalf("sendMedia voice returned %T", res)
	}
	return id
}
