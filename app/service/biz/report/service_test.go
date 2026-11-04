package report

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

type noteBox struct {
	msgs []sent
}

type sent struct {
	To   int64
	Text string
}

func (b *noteBox) Send(ctx context.Context, to int64, text string) error {
	b.msgs = append(b.msgs, sent{To: to, Text: text})
	return nil
}

func (b *noteBox) to(id int64) []string {
	var out []string
	for _, m := range b.msgs {
		if m.To == id {
			out = append(out, m.Text)
		}
	}
	return out
}

type mapLang map[int64]string

func (m mapLang) Lang(ctx context.Context, userID int64) string { return m[userID] }

type memObj struct {
	objs map[string][]byte
	fail bool
}

func (o *memObj) Put(ctx context.Context, key string, body []byte, contentType string) error {
	if !strings.HasPrefix(key, "reports/") {
		return errors.New("prefix")
	}
	if o.fail {
		return errors.New("down")
	}
	if o.objs == nil {
		o.objs = map[string][]byte{}
	}
	o.objs[key] = append([]byte(nil), body...)
	return nil
}

type fakeLLM struct {
	reply  string
	err    error
	calls  int
	system string
}

func (f *fakeLLM) CompletePrompt(ctx context.Context, model, system, prompt string) (string, int, error) {
	f.calls++
	f.system = system
	if f.err != nil {
		return "", 0, f.err
	}
	return f.reply, 3, nil
}

func testService(t *testing.T) (*Service, *noteBox, *memoryStore) {
	t.Helper()
	mem := NewMemory().(*memoryStore)
	s := New(mem)
	s.clock = func() int32 { return 1_700_000_000 }
	s.limit = 100
	box := &noteBox{}
	s.SetNotifier(box)
	return s, box, mem
}

func TestEachKindSurvivesReload(t *testing.T) {
	s, box, mem := testService(t)
	ctx := context.Background()
	kinds := []Submission{
		{Source: SourceStory, Reporter: 11, PeerType: mtproto.PEER_USER, PeerID: 21, SubjectUserID: 21, Reason: "spam", ObjectIDs: []int32{4, 2}, Lang: "zh-hans",
			Evidence: Evidence{Items: []Content{{Kind: "story", ID: 4, Text: "story text", MediaKeys: []string{"photos/9"}}}}},
		{Source: SourceMessages, Reporter: 11, PeerType: mtproto.PEER_USER, PeerID: 22, SubjectUserID: 22, Reason: "violence", ObjectIDs: []int32{8}, Message: "note", Lang: "en"},
		{Source: SourceReaction, Reporter: 11, PeerType: mtproto.PEER_USER, PeerID: 22, ObjectIDs: []int32{8}, ReactionPeerID: 30, SubjectUserID: 30, Lang: "en"},
		{Source: SourcePeer, Reporter: 11, PeerType: mtproto.PEER_USER, PeerID: 23, SubjectUserID: 23, Reason: "inputReportReasonSpam", Lang: "zh-hans"},
		{Source: SourceProfilePhoto, Reporter: 11, PeerType: mtproto.PEER_USER, PeerID: 24, SubjectUserID: 24, PhotoID: 99, Reason: "spam", Lang: "en",
			Evidence: Evidence{Items: []Content{{Kind: "photo", ID: 99, MediaKeys: []string{AvatarKey(99)}}}}},
		{Source: SourceSpam, Reporter: 11, PeerType: mtproto.PEER_USER, PeerID: 25, SubjectUserID: 25, Reason: "spam", Lang: "en"},
		{Source: SourceChannelSpam, Reporter: 11, PeerType: mtproto.PEER_CHANNEL, PeerID: 70, SubjectUserID: 26, ObjectIDs: []int32{3}, Reason: "spam", Lang: "en",
			Evidence: Evidence{Items: []Content{{Kind: "message", ID: 3, Text: "channel spam", MediaKeys: []string{"videos/5"}}}}},
	}
	for _, in := range kinds {
		row, err := s.Submit(ctx, in)
		if err != nil {
			t.Fatalf("%s: %v", in.Source, err)
		}
		if row.Status != StatusNeedsHuman || !strings.HasPrefix(row.Ticket(), "T") {
			t.Fatalf("%s row %+v", in.Source, row)
		}
	}
	if len(box.to(11)) != len(kinds) {
		t.Fatalf("receipts %d", len(box.to(11)))
	}
	for _, text := range box.to(11) {
		if !strings.Contains(text, "T") || strings.Contains(strings.ToLower(text), "ai") {
			t.Fatalf("receipt %q", text)
		}
	}
	raw, err := mem.marshalState()
	if err != nil {
		t.Fatal(err)
	}
	reloaded := NewMemory().(*memoryStore)
	if err = reloaded.unmarshalState(raw); err != nil {
		t.Fatal(err)
	}
	next := New(reloaded)
	got, err := next.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(kinds) {
		t.Fatalf("reloaded %d", len(got))
	}
	seen := map[string]bool{}
	for _, row := range got {
		seen[row.Source] = true
		again, err := next.Get(ctx, row.ID)
		if err != nil || again == nil || again.Source != row.Source || again.TargetKey != row.TargetKey {
			t.Fatalf("get %+v %v", again, err)
		}
	}
	for _, in := range kinds {
		if !seen[in.Source] {
			t.Fatalf("missing %s", in.Source)
		}
	}
	pending, err := next.Pending(ctx, 20)
	if err != nil || len(pending) != len(kinds) {
		t.Fatalf("pending %d %v", len(pending), err)
	}
}

func TestSnapshotMockAndPrefix(t *testing.T) {
	s, _, _ := testService(t)
	obj := &memObj{}
	s.SetObjects(obj)
	ctx := context.Background()
	row, err := s.Submit(ctx, Submission{
		Source: SourcePost, Reporter: 424242, PeerID: 77, Lang: "zh-hans",
		Evidence: Evidence{
			Items: []Content{{Kind: "message", ID: 4, Text: "hello msg", MediaKeys: []string{"photos/9", "documents/2"}}},
			Post:  &Content{Kind: "post", ID: 77, Text: "post body", MediaKeys: []string{"videos/5"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.EvidenceKey != "reports/"+itoa(row.ID)+".json" {
		t.Fatalf("key %s", row.EvidenceKey)
	}
	body := obj.objs[row.EvidenceKey]
	if !strings.Contains(string(body), "hello msg") || !strings.Contains(string(body), "photos/9") || !strings.Contains(string(body), "post body") || !strings.Contains(string(body), "videos/5") {
		t.Fatalf("snapshot %s", body)
	}
	if strings.Contains(string(body), "424242") {
		t.Fatalf("snapshot names the reporter: %s", body)
	}
	var doc snapDoc
	if err = json.Unmarshal(body, &doc); err != nil || doc.Ticket != row.Ticket() {
		t.Fatalf("doc %+v %v", doc, err)
	}

	bad := &memObj{fail: true}
	s.SetObjects(bad)
	row, err = s.Submit(ctx, Submission{Source: SourceSpam, Reporter: 12, PeerType: mtproto.PEER_USER, PeerID: 2, SubjectUserID: 2})
	if err != nil || row.EvidenceKey != "" {
		t.Fatalf("failed snapshot still stored %+v %v", row, err)
	}
	if err = (r2Store{}).Put(ctx, "other/1.json", []byte("x"), "application/json"); err == nil {
		t.Fatal("prefix accepted")
	}
}

func TestRateLimitPerReporter(t *testing.T) {
	s, _, _ := testService(t)
	s.limit = 2
	s.window = 100
	now := int32(1_700_000_000)
	s.clock = func() int32 { return now }
	ctx := context.Background()
	sub := func(reporter int64) error {
		_, err := s.Submit(ctx, Submission{Source: SourcePeer, Reporter: reporter, PeerType: mtproto.PEER_USER, PeerID: reporter + 1, SubjectUserID: reporter + 1})
		return err
	}
	if err := sub(1); err != nil {
		t.Fatal(err)
	}
	if err := sub(1); err != nil {
		t.Fatal(err)
	}
	err := sub(1)
	if err == nil || !strings.Contains(err.Error(), "FLOOD_WAIT") {
		t.Fatalf("limit: %v", err)
	}
	if err = sub(2); err != nil {
		t.Fatalf("other reporter: %v", err)
	}
	now += 101
	if err = sub(1); err != nil {
		t.Fatalf("window reset: %v", err)
	}
}

func TestDecisionMergeHidesReporter(t *testing.T) {
	s, box, _ := testService(t)
	s.SetLangs(mapLang{909090: "zh-hans"})
	ctx := context.Background()
	first, err := s.Submit(ctx, Submission{
		Source: SourcePeer, Reporter: 424242, Lang: "en",
		PeerType: mtproto.PEER_USER, PeerID: 909090, SubjectUserID: 909090, Reason: "spam",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Submit(ctx, Submission{
		Source: SourcePeer, Reporter: 424242, Lang: "en",
		PeerType: mtproto.PEER_USER, PeerID: 909090, SubjectUserID: 909090, Reason: "violence",
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := s.Submit(ctx, Submission{
		Source: SourcePeer, Reporter: 515151, Lang: "zh-hant",
		PeerType: mtproto.PEER_USER, PeerID: 909090, SubjectUserID: 909090, Reason: "spam",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.TargetKey != second.TargetKey || second.TargetKey != third.TargetKey {
		t.Fatalf("keys %s %s %s", first.TargetKey, second.TargetKey, third.TargetKey)
	}
	if len(box.to(909090)) != 0 {
		t.Fatal("subject heard about an open report")
	}
	if err = s.Decide(ctx, first.ID, StatusViolation); err != nil {
		t.Fatal(err)
	}
	subj := box.to(909090)
	if len(subj) != 1 {
		t.Fatalf("subject notices %d", len(subj))
	}
	if !strings.Contains(subj[0], "经平台审核") || strings.Contains(strings.ToLower(subj[0]), "ai") {
		t.Fatalf("subject text %q", subj[0])
	}
	if strings.Contains(subj[0], "424242") || strings.Contains(subj[0], "515151") || strings.Contains(subj[0], first.Ticket()) {
		t.Fatalf("subject text names a reporter: %q", subj[0])
	}
	rep1 := box.to(424242)
	rep2 := box.to(515151)
	if len(rep1) != 3 || len(rep2) != 2 {
		t.Fatalf("reporter notices %d %d", len(rep1), len(rep2))
	}
	merged := rep1[2]
	if !strings.Contains(merged, first.Ticket()) || !strings.Contains(merged, second.Ticket()) || strings.Contains(merged, third.Ticket()) || !strings.Contains(merged, "platform review") {
		t.Fatalf("merged en %q", merged)
	}
	if !strings.Contains(rep2[1], "經平台審核") || !strings.Contains(rep2[1], third.Ticket()) || strings.Contains(rep2[1], first.Ticket()) {
		t.Fatalf("hant %q", rep2[1])
	}
	if err = s.Decide(ctx, first.ID, StatusViolation); err != nil {
		t.Fatal(err)
	}
	if len(box.to(909090)) != 1 {
		t.Fatal("subject was notified twice")
	}
	if err = s.Decide(ctx, first.ID, StatusClear); err != ErrConflict {
		t.Fatalf("conflict %v", err)
	}
}

func TestClearNotifiesReporterOnly(t *testing.T) {
	s, box, _ := testService(t)
	ctx := context.Background()
	row, err := s.Submit(ctx, Submission{
		Source: SourceMessages, Reporter: 7, Lang: "zh-hans",
		PeerType: mtproto.PEER_USER, PeerID: 8, SubjectUserID: 8, ObjectIDs: []int32{1}, Reason: "other", Message: "no",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Decide(ctx, row.ID, StatusClear); err != nil {
		t.Fatal(err)
	}
	if len(box.to(8)) != 0 {
		t.Fatalf("subject got a clear notice: %+v", box.to(8))
	}
	got := box.to(7)
	if len(got) != 2 || !strings.Contains(got[1], "经平台审核") || !strings.Contains(got[1], "未发现违规") {
		t.Fatalf("reporter %q", got)
	}
}

func TestReviewerThreeResultsAndDefaultQueue(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		reply   string
		want    string
		subject bool
	}{
		{`{"result":"violation"}`, StatusViolation, true},
		{`{"result":"clear"}`, StatusClear, false},
		{`{"result":"needs_human"}`, StatusNeedsHuman, false},
	} {
		s, box, _ := testService(t)
		llm := &fakeLLM{reply: tc.reply}
		s.SetReviewer(NewReviewer(llm))
		row, err := s.Submit(ctx, Submission{
			Source: SourceStory, Reporter: 3, Lang: "en",
			PeerType: mtproto.PEER_USER, PeerID: 4, SubjectUserID: 4, ObjectIDs: []int32{1}, Reason: "spam",
			Evidence: Evidence{Items: []Content{{Kind: "story", Text: "caption"}}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if llm.calls != 1 || row.Status != tc.want {
			t.Fatalf("%s calls %d status %s", tc.reply, llm.calls, row.Status)
		}
		if strings.Contains(strings.ToLower(llm.system), "ai") {
			t.Fatalf("system %s", llm.system)
		}
		if tc.subject && len(box.to(4)) != 1 {
			t.Fatalf("subject %d for %s", len(box.to(4)), tc.reply)
		}
		if !tc.subject && len(box.to(4)) != 0 {
			t.Fatalf("subject notified for %s", tc.reply)
		}
		if tc.want == StatusNeedsHuman {
			pending, err := s.Pending(ctx, 10)
			if err != nil || len(pending) != 1 {
				t.Fatalf("queue %+v %v", pending, err)
			}
		}
	}

	s, _, _ := testService(t)
	llm := &fakeLLM{}
	row, err := s.Submit(ctx, Submission{Source: SourcePeer, Reporter: 5, PeerType: mtproto.PEER_USER, PeerID: 6, SubjectUserID: 6})
	if err != nil || row.Status != StatusNeedsHuman || llm.calls != 0 {
		t.Fatalf("off %+v calls %d %v", row, llm.calls, err)
	}
	pending, err := s.Pending(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].ID != row.ID {
		t.Fatalf("default queue %+v %v", pending, err)
	}

	s.SetReviewer(NewReviewer(&fakeLLM{err: errors.New("down")}))
	row, err = s.Submit(ctx, Submission{Source: SourcePeer, Reporter: 5, PeerType: mtproto.PEER_CHAT, PeerID: 9})
	if err != nil || row.Status != StatusNeedsHuman {
		t.Fatalf("model error %+v %v", row, err)
	}
}

func TestAutoReviewMergesSubjectNotice(t *testing.T) {
	s, box, _ := testService(t)
	s.SetReviewer(NewReviewer(&fakeLLM{reply: `{"result":"violation"}`}))
	ctx := context.Background()
	if _, err := s.Submit(ctx, Submission{Source: SourcePeer, Reporter: 41, Lang: "en", PeerType: mtproto.PEER_USER, PeerID: 42, SubjectUserID: 42, Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, Submission{Source: SourcePeer, Reporter: 43, Lang: "zh-hans", PeerType: mtproto.PEER_USER, PeerID: 42, SubjectUserID: 42, Reason: "spam"}); err != nil {
		t.Fatal(err)
	}
	if len(box.to(42)) != 1 || strings.Contains(box.to(42)[0], "41") || strings.Contains(box.to(42)[0], "43") {
		t.Fatalf("subject %+v", box.to(42))
	}
	if len(box.to(41)) != 2 || len(box.to(43)) != 2 {
		t.Fatalf("reporters %d %d", len(box.to(41)), len(box.to(43)))
	}
}

func TestCopyNeverNamesAModel(t *testing.T) {
	for _, lang := range []string{"zh-hans", "zh-hant", "en", "ja", "zh-CN", "zh-TW", ""} {
		samples := []string{
			Receipt(lang, "T12", etaText(24, lang)),
			Receipt(lang, "T1", etaText(1, lang)),
			DecisionForReporter(lang, StatusViolation, JoinTickets([]int64{2, 1}, lang)),
			DecisionForReporter(lang, StatusClear, "T3"),
			DecisionForSubject(lang, StatusViolation),
			DecisionForSubject(lang, StatusClear),
			reviewSystem,
		}
		for _, text := range samples {
			if strings.Contains(strings.ToLower(text), "ai") {
				t.Fatalf("lang %q contains AI: %s", lang, text)
			}
		}
	}
	if !strings.Contains(Receipt("zh-hans", "T1", etaText(24, "zh-hans")), "经平台审核") {
		t.Fatal("simplified receipt")
	}
	if !strings.Contains(DecisionForReporter("zh-hans", StatusViolation, "T1"), "经平台审核") {
		t.Fatal("simplified decision")
	}
	if !strings.Contains(DecisionForSubject("zh-CN", StatusViolation), "经平台审核") {
		t.Fatal("simplified subject")
	}
	if !strings.Contains(DecisionForReporter("zh-TW", StatusClear, "T1"), "經平台審核") {
		t.Fatal("traditional clear")
	}
	if !strings.Contains(Receipt("zh-Hant", "T8", etaText(3, "zh-HK")), "經平台審核") {
		t.Fatal("traditional receipt")
	}
	if DecisionForSubject("en", StatusClear) != "" {
		t.Fatal("clear subject text")
	}
	if JoinTickets([]int64{3, 1}, "zh-hans") != "T1、T3" {
		t.Fatalf("tickets %s", JoinTickets([]int64{3, 1}, "zh-hans"))
	}
}

func TestParseVerdict(t *testing.T) {
	cases := map[string]string{
		`{"result":"violation"}`:               StatusViolation,
		"```json\n{\"result\":\"clear\"}\n```": StatusClear,
		`{"result":"needs_human"}`:             StatusNeedsHuman,
		`{"result":"no_violation"}`:            StatusClear,
		"not sure":                             StatusNeedsHuman,
		"":                                     StatusNeedsHuman,
		"violation":                            StatusViolation,
	}
	for in, want := range cases {
		if got := parseVerdict(in); got != want {
			t.Fatalf("%q -> %s want %s", in, got, want)
		}
	}
}

func TestStoryBackfillKey(t *testing.T) {
	row := RowFromStoryReport(8, 3, 1, 9, "5,4", "spam", "x", 10)
	if row.TargetKey != "story:2:9:4,5" || row.LegacyStoryID != 8 || row.Source != SourceStory || row.Status != StatusNeedsHuman || row.SubjectUserID != 9 {
		t.Fatalf("%+v", row)
	}
	ch := RowFromStoryReport(9, 3, 2, 77, "1", "other", "", 10)
	if ch.PeerType != mtproto.PEER_CHANNEL || ch.SubjectUserID != 0 || ch.TargetKey != "story:4:77:1" {
		t.Fatalf("%+v", ch)
	}
}

func TestMediaKeys(t *testing.T) {
	photo := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{
		Photo_FLAGPHOTO: mtproto.MakeTLPhoto(&mtproto.Photo{Id: 9}).To_Photo(),
	}).To_MessageMedia()
	if got := MediaKeys(photo); len(got) != 1 || got[0] != "photos/9" {
		t.Fatalf("photo %v", got)
	}
	video := mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Document: mtproto.MakeTLDocument(&mtproto.Document{Id: 5, MimeType: "video/mp4"}).To_Document(),
	}).To_MessageMedia()
	if got := MediaKeys(video); len(got) != 1 || got[0] != "videos/5" {
		t.Fatalf("video %v", got)
	}
	msg := ContentFromMessage(4, mtproto.MakeTLMessage(&mtproto.Message{Id: 4, Message: "hi", Media: photo}).To_Message())
	if msg.Text != "hi" || len(msg.MediaKeys) != 1 {
		t.Fatalf("%+v", msg)
	}
	raw := []byte(`{"message":"json","media":{"document":{"id":8,"mime_type":"image/gif"},"photo_FLAGPHOTO":{"id":3}}}`)
	got := contentFromMessageJSON(1, "", raw)
	if got.Text != "json" || len(got.MediaKeys) != 2 || got.MediaKeys[0] != "photos/3" || got.MediaKeys[1] != "documents/8" {
		t.Fatalf("%+v", got)
	}
	if AvatarKey(4) != "avatars/4" || AvatarKey(0) != "" {
		t.Fatal("avatar")
	}
}

func TestStartDefaultsOff(t *testing.T) {
	t.Setenv(EnvReview, "")
	t.Setenv(EnvSnapshot, "")
	t.Setenv(EnvHTTP, "")
	t.Setenv("TEAMGRAM_MYSQL_DSN", "")
	ResetShared()
	Start("", nil)
	s := Shared()
	if s.reviewer != nil || s.objects != nil {
		t.Fatal("switches default on")
	}
}
