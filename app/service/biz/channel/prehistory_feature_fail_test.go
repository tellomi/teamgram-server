package channel

import (
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestGetFeaturesFailureDoesNotLeakPrehistory is the in-memory case that
// failed review of 00be729df. Three pre-join posts exist. The creator's
// counter is 3. A new member must not be given that count, the discussion
// body, or the replies when GetFeatures fails. The gate is unknown.
// This is not a MySQL or MTProto run.
func TestGetFeaturesFailureDoesNotLeakPrehistory(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "FeatureFail")
	in := inCh(ch)
	const (
		owner  int64 = 1
		member int64 = 2
		when   int32 = 1_700_000_000
	)
	rootSent, err := svc.Send(ctx, owner, ch.Id, 0, []Outgoing{{RandomID: 910188001, Message: &mtproto.Message{
		Message: "probe-secret",
		Date:    when,
	}}})
	if err != nil {
		t.Fatal(err)
	}
	rootID := rootSent.Updates[0].GetMessage_MESSAGE().GetId()
	replyTo := mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: rootID}).To_MessageReplyHeader()
	for i, text := range []string{"probe-secret-reply", "probe-secret-tail"} {
		if _, err = svc.Send(ctx, owner, ch.Id, 0, []Outgoing{{RandomID: int64(910188002 + i), Message: &mtproto.Message{
			Message: text,
			Date:    when,
			ReplyTo: replyTo,
		}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = svc.TogglePreHistory(ctx, owner, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if err = svc.store.PutParticipant(ctx, &Participant{
		ChannelID: ch.Id,
		UserID:    member,
		Role:      RoleMember,
		State:     StateNormal,
		Date:      when + 100,
	}); err != nil {
		t.Fatal(err)
	}
	// Drop the create-service row so the only live posts are the three
	// pre-join bodies. The unfixed member call returned count=3 and no error.
	live, err := svc.store.ListMessages(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	var drop []int32
	for _, stored := range live {
		body, uerr := unmarshalMessage(stored.Data)
		if uerr != nil || body == nil || !strings.Contains(body.GetMessage(), "probe-secret") {
			drop = append(drop, stored.ID)
		}
	}
	if _, _, err = svc.Delete(ctx, owner, ch.Id, 0, drop); err != nil {
		t.Fatal(err)
	}

	creatorCount, err := svc.CountByFilter(ctx, owner, ch.Id, nil)
	if err != nil || creatorCount != 3 {
		t.Fatalf("creator count = %d, %v; the leaked success was count=3", creatorCount, err)
	}
	memberCount, err := svc.CountByFilter(ctx, member, ch.Id, nil)
	if err != nil || memberCount != 0 {
		t.Fatalf("member count before the fault = %d, %v", memberCount, err)
	}
	if _, err = svc.DiscussionMessage(ctx, member, ch.Id, rootID); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("member discussion before the fault: %v", err)
	}
	replies, err := svc.Replies(ctx, member, ch.Id, rootID, HistoryQuery{Limit: 20})
	if err != nil || textsContain(replies.GetMessages(), "probe-secret") {
		t.Fatalf("member replies before the fault: %v %+v", err, replies)
	}
	gate := svc.PrehistoryGate(ctx, member, ch.Id)
	if gate.Unknown || !gate.Hidden || !gate.Blocks(when) {
		t.Fatalf("gate before the fault: %+v", gate)
	}

	base := svc.store
	svc.store = &failFeatures{Store: base}
	defer func() { svc.store = base }()

	t.Run("discussion", func(t *testing.T) {
		got, err := svc.DiscussionMessage(ctx, member, ch.Id, rootID)
		if err == nil || (got != nil && textsContain(got.GetMessages(), "probe-secret")) {
			t.Fatalf("discussion returned the pre-join body: %v %+v", err, got)
		}
	})
	t.Run("replies", func(t *testing.T) {
		got, err := svc.Replies(ctx, member, ch.Id, rootID, HistoryQuery{Limit: 20})
		if err == nil || (got != nil && textsContain(got.GetMessages(), "probe-secret")) {
			t.Fatalf("replies returned the pre-join body: %v %+v", err, got)
		}
	})
	t.Run("count", func(t *testing.T) {
		n, err := svc.CountByFilter(ctx, member, ch.Id, nil)
		if err == nil {
			t.Fatalf("CountByFilter returned count=%d with no error", n)
		}
	})
	t.Run("gate", func(t *testing.T) {
		g := svc.PrehistoryGate(ctx, member, ch.Id)
		if !g.Unknown || !g.Hidden || !g.Blocks(when) || !g.Blocks(when+100) {
			t.Fatalf("feature read was not unknown: %+v", g)
		}
	})
}

// TestFeatureReadFailureIsNotWrittenEmpty covers the writers that would
// otherwise persist the empty object features() used for a failed read.
// channels.togglePreHistoryHidden and channels.deactivateAllUsernames share
// channel_feature with hidden prehistory and usernames.
func TestFeatureReadFailureIsNotWrittenEmpty(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "KeepFeatures")
	in := inCh(ch)
	if _, err := svc.TogglePreHistory(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	kept, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	kept.SlowmodeSeconds = 30
	kept.Usernames = []UsernameRec{
		{Username: "mainname", Active: true, Editable: true},
		{Username: "collectible", Active: true, Editable: false},
	}
	kept.JoinTop = map[int64]int32{2: 9}
	if err = svc.saveFeatures(ctx, ch.Id, kept); err != nil {
		t.Fatal(err)
	}

	base := svc.store
	svc.store = &failFeatures{Store: base}
	blank := svc.features(ctx, ch.Id)
	if !blank.loadFailed {
		t.Fatal("features() treated a failed read as an empty config")
	}
	blank.HiddenPrehistory = false
	blank.Usernames = nil
	blank.SlowmodeSeconds = 0
	if err = svc.saveFeatures(ctx, ch.Id, blank); err == nil {
		t.Fatal("saveFeatures persisted a failed read")
	}
	if _, err = svc.TogglePreHistory(ctx, 1, 0, in, false); err == nil {
		t.Fatal("togglePreHistory wrote after a failed feature read")
	}
	if _, err = svc.DeactivateAllUsernames(ctx, 1, 0, in); err == nil {
		t.Fatal("deactivateAllUsernames treated a failed feature read as success")
	}
	svc.store = base

	got, err := svc.mutableFeatures(ctx, ch.Id)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HiddenPrehistory || got.SlowmodeSeconds != 30 || got.JoinTop[2] != 9 {
		t.Fatalf("failed read was written back: %+v", got)
	}
	if len(got.Usernames) != 2 || got.Usernames[1].Username != "collectible" || !got.Usernames[1].Active {
		t.Fatalf("usernames were replaced: %+v", got.Usernames)
	}
}

func TestDecodeFeaturePayload(t *testing.T) {
	scanErr := errors.New("scan failed")
	cases := []struct {
		name    string
		data    []byte
		err     error
		wantErr error
		hidden  bool
	}{
		{name: "no rows", err: sql.ErrNoRows},
		{name: "scan", data: nil, err: scanErr, wantErr: scanErr},
		{name: "empty", data: []byte("  "), wantErr: errEmptyFeaturePayload},
		{name: "null", data: []byte("null"), wantErr: errEmptyFeaturePayload},
		{name: "bad json", data: []byte("{"), wantErr: errAny},
		{name: "empty object"},
		{name: "hidden", data: []byte(`{"HiddenPrehistory":true}`), hidden: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data := tc.data
			if tc.name == "empty object" {
				data = []byte("{}")
			}
			got, err := decodeFeaturePayload(data, tc.err)
			if tc.wantErr == errAny {
				if err == nil {
					t.Fatal("bad json was accepted")
				}
				return
			}
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) && (err == nil || err.Error() != tc.wantErr.Error()) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				if got != nil && tc.wantErr != sql.ErrNoRows {
					t.Fatalf("payload returned with error: %+v", got)
				}
				return
			}
			if err != nil || got == nil || got.HiddenPrehistory != tc.hidden {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}

var errAny = errors.New("any")

func textsContain(msgs []*mtproto.Message, needle string) bool {
	for _, m := range msgs {
		if m != nil && strings.Contains(m.GetMessage(), needle) {
			return true
		}
	}
	return false
}
