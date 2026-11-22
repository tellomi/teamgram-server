package e2e

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// prehistoryFault is set by the isolated-stack wrapper. CI's compose run
// leaves it nil: the member-read subtest does not pretend to have failed a
// lookup. The other subtests still speak MTProto.
var prehistoryFault *participantFaultProxy

// TestHiddenPrehistoryProtocol speaks MTProto to a Teamgram stack.
// Phones 9996610911–9996610913, code 11111, layer 232. Accounts are the
// synthetic 99966 test range. The assertions are the RPC bodies and the
// read-participant vector, not the in-memory channel tests.
func TestHiddenPrehistoryProtocol(t *testing.T) {
	cfg := requireE2E(t)
	db := openPrehistoryDB(t)
	ctx, cancel := contextTimeout(4 * time.Minute)
	defer cancel()

	owner, _ := dialPhone(t, ctx, cfg, "9996610911", "PreOwn")
	older, _ := dialPhone(t, ctx, cfg, "9996610912", "PreOld")
	newbie, _ := dialPhone(t, ctx, cfg, "9996610913", "PreNew")
	olderID, olderHash, err := owner.ImportPhone(ctx, "9996610912", "PreOld")
	if err != nil {
		t.Fatal(err)
	}
	newbieID, newbieHash, err := owner.ImportPhone(ctx, "9996610913", "PreNew")
	if err != nil {
		t.Fatal(err)
	}

	groupID, groupHash, err := owner.CreateMegagroup(ctx, fmt.Sprintf("prehist-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	group := inputChannel(groupID, groupHash)
	if err = owner.InviteToChannel(ctx, group, inputUser(olderID, olderHash)); err != nil {
		t.Fatal(err)
	}
	olderCh := waitChannel(t, ctx, older, groupID)
	// The earlier member has to be in the group before the secret, and not
	// in the same second, or the join-top cutoff would hide it from them too.
	time.Sleep(1100 * time.Millisecond)
	ownerPeer := inputPeerChannel(groupID, groupHash)
	secret := fmt.Sprintf("old-secret-proto-%d", time.Now().UnixNano())
	mention := mtproto.MakeTLMessageEntityMentionName(&mtproto.MessageEntity{
		Offset:       0,
		Length:       int32(len(secret)),
		UserId_INT64: newbieID,
	}).To_MessageEntity()
	oldID, err := owner.SendText(ctx, ownerPeer, secret, []*mtproto.MessageEntity{mention})
	if err != nil || oldID == 0 {
		t.Fatalf("send secret id=%d err=%v", oldID, err)
	}
	pinMessage(t, owner, ownerPeer, oldID, false)
	if _, err = owner.Invoke(ctx, &mtproto.TLChannelsTogglePreHistoryHidden{
		Constructor: mtproto.CRC32_channels_togglePreHistoryHidden,
		Channel:     group,
		Enabled:     mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	if err = owner.InviteToChannel(ctx, group, inputUser(newbieID, newbieHash)); err != nil {
		t.Fatal(err)
	}
	newbieCh := waitChannel(t, ctx, newbie, groupID)
	newbiePeer := inputPeerChannel(groupID, newbieCh.GetAccessHash())
	visible := fmt.Sprintf("new-open-proto-%d", time.Now().UnixNano())
	freshID, err := owner.SendText(ctx, ownerPeer, visible, nil)
	if err != nil || freshID == 0 || freshID <= oldID {
		t.Fatalf("post-join id=%d old=%d err=%v", freshID, oldID, err)
	}
	top := joinTopOf(t, db, groupID, newbieID)
	if top < oldID || top >= freshID {
		t.Fatalf("join top %d does not sit between secret %d and post-join %d", top, oldID, freshID)
	}
	inbox := readInboxOf(t, db, groupID, newbieID)
	if inbox < oldID {
		t.Fatalf("join left inbox cursor %d behind secret %d, so a read-list miss would be inconclusive", inbox, oldID)
	}
	olderPeer := inputPeerChannel(groupID, olderCh.GetAccessHash())
	if !textIn(historyMessages(t, older, ctx, olderPeer), secret) {
		t.Fatal("member who joined before the secret cannot read it")
	}

	t.Run("read_paths", func(t *testing.T) {
		hist, count := historyResult(t, newbie, ctx, newbiePeer)
		if textIn(hist, secret) {
			t.Fatalf("getHistory returned the pre-join body: %v", protoTexts(hist))
		}
		if !textIn(hist, visible) {
			t.Fatalf("getHistory dropped the post-join body: %v", protoTexts(hist))
		}
		t.Logf("messages.getHistory count=%d visible=%d (count may include hidden rows)", count, len(protoTexts(hist)))
		byID := channelMessages(t, newbie, ctx, newbieCh, oldID, freshID)
		if !messageEmpty(byID, oldID) || textIn(byID, secret) {
			t.Fatalf("channels.getMessages returned the pre-join body: %v", protoTexts(byID))
		}
		if !textIn(byID, visible) {
			t.Fatalf("channels.getMessages dropped the post-join body: %v", protoTexts(byID))
		}
		if textIn(searchMessages(t, newbie, ctx, newbiePeer, secret, mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter()), secret) {
			t.Fatal("messages.search returned the pre-join body")
		}
		if textIn(searchMessages(t, newbie, ctx, newbiePeer, "", pinnedFilter()), secret) {
			t.Fatal("pinned messages.search returned the pre-join body")
		}
		diff, err := channelDifference(ctx, newbie, newbieCh, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if _, tooLong := diff.(*mtproto.TLUpdatesChannelDifferenceTooLong); tooLong {
			t.Fatalf("pts log still present but updates.getChannelDifference returned %T", diff)
		}
		if textIn(differenceMessages(diff), secret) {
			t.Fatalf("updates.getChannelDifference returned the pre-join body: %v", protoTexts(differenceMessages(diff)))
		}
		if !textIn(differenceMessages(diff), visible) {
			t.Fatalf("updates.getChannelDifference dropped the post-join body: %v", protoTexts(differenceMessages(diff)))
		}
	})

	t.Run("read_list", func(t *testing.T) {
		ids, n := readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
		if containsID(ids, newbieID) {
			t.Fatalf("new member is on the pre-join read list before readHistory: %v", ids)
		}
		if err = older.ReadChannelHistory(ctx, olderCh, oldID); err != nil {
			t.Fatal(err)
		}
		ids, n = readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
		if !containsID(ids, olderID) {
			t.Fatalf("member who read the secret is missing: %v", ids)
		}
		if containsID(ids, newbieID) || n != 1 {
			t.Fatalf("read list count=%d ids=%v; new member must not raise it", n, ids)
		}
		if err = newbie.ReadChannelHistory(ctx, newbieCh, freshID); err != nil {
			t.Fatal(err)
		}
		ids, n = readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
		if containsID(ids, newbieID) || n != 1 {
			t.Fatalf("readHistory of a later message put the new member on the pre-join read list: count=%d ids=%v", n, ids)
		}
		freshIDs, freshN := readParticipantIDs(t, owner, ctx, ownerPeer, freshID)
		if !containsID(freshIDs, newbieID) {
			t.Fatalf("new member is not a reader of the visible message: %v", freshIDs)
		}
		if freshN < 1 {
			t.Fatalf("visible read list count %d", freshN)
		}
	})

	t.Run("same_second", func(t *testing.T) {
		how := alignJoinSecond(t, db, groupID, newbieID, oldID, freshID)
		t.Logf("same-second stimulus: %s", how)
		hist, _ := historyResult(t, newbie, ctx, newbiePeer)
		if textIn(hist, secret) || !textIn(hist, visible) {
			t.Fatalf("getHistory after same-second cutoff: %v", protoTexts(hist))
		}
		byID := channelMessages(t, newbie, ctx, newbieCh, oldID, freshID)
		if !messageEmpty(byID, oldID) || !textIn(byID, visible) {
			t.Fatalf("channels.getMessages after same-second cutoff: %v", protoTexts(byID))
		}
		diff, err := channelDifference(ctx, newbie, newbieCh, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		if textIn(differenceMessages(diff), secret) || !textIn(differenceMessages(diff), visible) {
			t.Fatalf("updates.getChannelDifference after same-second cutoff (%T): %v", diff, protoTexts(differenceMessages(diff)))
		}
	})

	t.Run("too_long", func(t *testing.T) {
		res, err := db.Exec(`DELETE FROM channel_pts_updates WHERE channel_id=?`, groupID)
		if err != nil {
			t.Fatal(err)
		}
		n, _ := res.RowsAffected()
		t.Logf("deleted %d pts rows to reach channelDifferenceTooLong", n)
		diff, err := channelDifference(ctx, newbie, newbieCh, 0, 100)
		if err != nil {
			t.Fatal(err)
		}
		long, ok := diff.(*mtproto.TLUpdatesChannelDifferenceTooLong)
		if !ok {
			t.Fatalf("updates.getChannelDifference returned %T, want channelDifferenceTooLong", diff)
		}
		if textIn(long.GetMessages(), secret) {
			t.Fatalf("channelDifferenceTooLong returned the pre-join body: %v", protoTexts(long.GetMessages()))
		}
		dlg := long.GetDialog()
		if dlg.GetTopMessage() == oldID {
			t.Fatalf("channelDifferenceTooLong dialog top is the hidden message %d", oldID)
		}
		t.Logf("tooLong dialog pts=%d top=%d read_inbox=%d (pts is the channel cursor, not a body)", dlg.GetPts().GetValue(), dlg.GetTopMessage(), dlg.GetReadInboxMaxId())
	})

	t.Run("count_and_pin", func(t *testing.T) {
		_, ownerCount := historyResultCtx(t, owner, ownerPeer)
		hist, count := historyResultCtx(t, newbie, newbiePeer)
		if textIn(hist, secret) {
			t.Fatalf("getHistory page still has the pre-join body: %v", protoTexts(hist))
		}
		if count != int32(len(hist)) {
			t.Fatalf("getHistory count %d is not the visible page %d", count, len(hist))
		}
		if count >= ownerCount {
			t.Fatalf("new member count %d is not below the owner's %d", count, ownerCount)
		}
		res, err := newbie.Invoke(ctx, &mtproto.TLMessagesGetHistory{
			Constructor: mtproto.CRC32_messages_getHistory,
			Peer:        newbiePeer,
			OffsetId:    freshID,
			Limit:       50,
		})
		if err != nil {
			t.Fatal(err)
		}
		if textIn(messagesOrEmpty(res), secret) {
			t.Fatal("getHistory offset page returned the pre-join body")
		}
		full := channelFull(t, ctx, newbie, newbieCh)
		if id := full.GetPinnedMsgId().GetValue(); id == oldID {
			t.Fatalf("channelFull still names the hidden pin %d", id)
		}
		ownerFull := channelFull(t, ctx, owner, group)
		if ownerFull.GetPinnedMsgId().GetValue() != oldID {
			t.Fatalf("owner lost the pin, got %d", ownerFull.GetPinnedMsgId().GetValue())
		}
	})

	t.Run("mentions", func(t *testing.T) {
		ownerMsgs := channelMessages(t, owner, ctx, group, oldID)
		if len(ownerMsgs) == 0 || len(ownerMsgs[0].GetEntities()) == 0 {
			t.Fatal("mention entity was not stored on the pre-join message")
		}
		if _, err := db.Exec(`UPDATE channel_read_state SET read_inbox_max_id=0 WHERE channel_id=? AND user_id=?`, groupID, newbieID); err != nil {
			t.Fatal(err)
		}
		defer func() {
			_, _ = db.Exec(`UPDATE channel_read_state SET read_inbox_max_id=? WHERE channel_id=? AND user_id=?`, freshID, groupID, newbieID)
		}()
		res, err := newbie.Invoke(ctx, &mtproto.TLMessagesGetUnreadMentions{
			Constructor: mtproto.CRC32_messages_getUnreadMentions_f107e790,
			Peer:        newbiePeer,
			Limit:       50,
		})
		if err != nil {
			t.Fatal(err)
		}
		if textIn(messagesOrEmpty(res), secret) {
			t.Fatalf("getUnreadMentions returned the pre-join body: %v", protoTexts(messagesOrEmpty(res)))
		}
	})

	if prehistoryFault != nil {
		t.Run("member_read_error", func(t *testing.T) {
			prehistoryFault.Arm(newbieID, 1)
			hist, err := newbie.Invoke(ctx, &mtproto.TLMessagesGetHistory{
				Constructor: mtproto.CRC32_messages_getHistory,
				Peer:        newbiePeer,
				Limit:       50,
			})
			prehistoryFault.Disarm()
			if textIn(messagesOrEmpty(hist), secret) {
				t.Fatalf("getHistory returned the body after a failed member read: %v\n%s", err, prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() < 1 {
				t.Fatalf("proxy did not fail a participant read during getHistory\n%s", prehistoryFault.LogText())
			}
			if err != nil {
				t.Logf("getHistory failed closed with %v", err)
			}
			before := prehistoryFault.Fails()
			prehistoryFault.Arm(newbieID, 1)
			got, err := newbie.Invoke(ctx, &mtproto.TLChannelsGetMessages{
				Constructor:           mtproto.CRC32_channels_getMessages_ad8c9a23,
				Channel:               newbieCh,
				Id_VECTORINPUTMESSAGE: inputMessageIDs(oldID),
			})
			prehistoryFault.Disarm()
			if textIn(messagesOrEmpty(got), secret) {
				t.Fatalf("channels.getMessages returned the body after a failed member read: %v\n%s", err, prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() == before {
				t.Fatalf("proxy did not fail a participant read during getMessages\n%s", prehistoryFault.LogText())
			}
			t.Logf("member read errors injected=%d last=%v", prehistoryFault.Fails(), err)
			before = prehistoryFault.Fails()
			prehistoryFault.Arm(newbieID, 1)
			diff, err := channelDifference(ctx, newbie, newbieCh, 0, 100)
			prehistoryFault.Disarm()
			if textIn(differenceMessages(diff), secret) {
				t.Fatalf("getChannelDifference returned the body after a failed member read: %v\n%s", err, prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() == before {
				t.Fatalf("proxy did not fail a participant read during getChannelDifference\n%s", prehistoryFault.LogText())
			}
			before = prehistoryFault.Fails()
			prehistoryFault.Arm(newbieID, 1)
			found := searchMessages(t, newbie, ctx, newbiePeer, secret, mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter())
			prehistoryFault.Disarm()
			if textIn(found, secret) {
				t.Fatalf("messages.search returned the body after a failed member read\n%s", prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() == before {
				t.Fatalf("proxy did not fail a participant read during messages.search\n%s", prehistoryFault.LogText())
			}
		})

		t.Run("join_save_failure", func(t *testing.T) {
			late, _ := dialPhone(t, ctx, cfg, "9996610914", "PreLate")
			lateID, lateHash, err := owner.ImportPhone(ctx, "9996610914", "PreLate")
			if err != nil {
				t.Fatal(err)
			}
			prehistoryFault.ArmFeatureWrite()
			err = owner.InviteToChannel(ctx, group, inputUser(lateID, lateHash))
			prehistoryFault.Disarm()
			if err == nil {
				t.Fatal("invite succeeded while the join top could not be saved")
			}
			var state int
			qerr := db.QueryRow(`SELECT state FROM channel_participants WHERE channel_id=? AND user_id=?`, groupID, lateID).Scan(&state)
			if qerr == sql.ErrNoRows || (qerr == nil && state != 0) {
				t.Logf("failed join left no active membership (%v state=%d)", qerr, state)
				return
			}
			if qerr != nil {
				t.Fatal(qerr)
			}
			res, herr := late.Invoke(ctx, &mtproto.TLMessagesGetHistory{
				Constructor: mtproto.CRC32_messages_getHistory,
				Peer:        inputPeerChannel(groupID, groupHash),
				Limit:       50,
			})
			if textIn(messagesOrEmpty(res), secret) {
				t.Fatalf("failed join still returned the pre-join body: %v", protoTexts(messagesOrEmpty(res)))
			}
			if herr != nil {
				t.Logf("history after failed join: %v", herr)
			}
		})

		t.Run("feature_read_failure", func(t *testing.T) {
			// GetFeatures must not be treated as an empty config. The same
			// SELECT feeds getDiscussionMessage, getReplies, and the search
			// counter. An error is closed. A body that contains the secret
			// is not.
			check := func(name string, call func() (mtproto.TLObject, error)) {
				t.Helper()
				before := prehistoryFault.Fails()
				prehistoryFault.ArmFeatureRead()
				obj, err := call()
				prehistoryFault.Disarm()
				if textIn(discussionOrMessages(obj), secret) {
					t.Fatalf("%s returned the pre-join body after a failed feature read: %v\n%s", name, err, prehistoryFault.LogText())
				}
				if prehistoryFault.Fails() == before {
					t.Fatalf("%s did not read channel_feature\n%s", name, prehistoryFault.LogText())
				}
				if err != nil {
					t.Logf("%s failed closed with %v", name, err)
				}
			}
			check("messages.getDiscussionMessage", func() (mtproto.TLObject, error) {
				return newbie.Invoke(ctx, &mtproto.TLMessagesGetDiscussionMessage{
					Constructor: mtproto.CRC32_messages_getDiscussionMessage,
					Peer:        newbiePeer,
					MsgId:       oldID,
				})
			})
			check("messages.getReplies", func() (mtproto.TLObject, error) {
				return newbie.Invoke(ctx, &mtproto.TLMessagesGetReplies{
					Constructor: mtproto.CRC32_messages_getReplies,
					Peer:        newbiePeer,
					MsgId:       oldID,
					Limit:       20,
				})
			})
			before := prehistoryFault.Fails()
			prehistoryFault.ArmFeatureRead()
			counterObj, counterErr := newbie.Invoke(ctx, &mtproto.TLMessagesGetSearchCounters{
				Constructor: mtproto.CRC32_messages_getSearchCounters_732eef00,
				Peer:        newbiePeer,
				Filters:     []*mtproto.MessagesFilter{mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter()},
			})
			prehistoryFault.Disarm()
			if counterErr == nil && searchCounterTotal(counterObj) > 0 && textIn(messagesOrEmpty(counterObj), secret) {
				t.Fatalf("messages.getSearchCounters returned the pre-join body\n%s", prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() == before {
				t.Fatalf("messages.getSearchCounters did not read channel_feature\n%s", prehistoryFault.LogText())
			}
			if counterErr == nil {
				t.Fatalf("messages.getSearchCounters returned count=%d with no error while channel_feature could not be read", searchCounterTotal(counterObj))
			}
			t.Logf("messages.getSearchCounters failed closed with %v", counterErr)
		})

		t.Run("read_list_feature_failure", func(t *testing.T) {
			before := prehistoryFault.Fails()
			prehistoryFault.ArmFeatureRead()
			obj, err := invokeReadParticipants(owner, ownerPeer, oldID)
			prehistoryFault.Disarm()
			if err == nil || containsID(readerIDsOf(obj), newbieID) {
				t.Fatalf("getMessageReadParticipants returned a list after a failed feature read: %v ids=%v\n%s", err, readerIDsOf(obj), prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() == before {
				t.Fatalf("getMessageReadParticipants did not read channel_feature\n%s", prehistoryFault.LogText())
			}
			ids, _ := readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
			if containsID(ids, newbieID) {
				t.Fatalf("feature read failure left the join cursor on the read list: %v", ids)
			}
		})

		t.Run("read_list_participant_failure", func(t *testing.T) {
			before := prehistoryFault.Fails()
			prehistoryFault.ArmParticipantList()
			obj, err := invokeReadParticipants(owner, ownerPeer, oldID)
			prehistoryFault.Disarm()
			if err == nil || containsID(readerIDsOf(obj), newbieID) {
				t.Fatalf("getMessageReadParticipants returned a list after a failed participant list: %v ids=%v\n%s", err, readerIDsOf(obj), prehistoryFault.LogText())
			}
			if prehistoryFault.Fails() == before {
				t.Fatalf("getMessageReadParticipants did not list channel_participants\n%s", prehistoryFault.LogText())
			}
			ids, _ := readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
			if containsID(ids, newbieID) || !containsID(ids, olderID) {
				t.Fatalf("participant list failure changed the stored read list: %v", ids)
			}
		})

		t.Run("dual_persist_failure", func(t *testing.T) {
			dual, _ := dialPhone(t, ctx, cfg, "9996610915", "PreDual")
			dualID, dualHash, err := owner.ImportPhone(ctx, "9996610915", "PreDual")
			if err != nil {
				t.Fatal(err)
			}
			before := prehistoryFault.Fails()
			prehistoryFault.ArmReadWrite()
			prehistoryFault.ArmParticipantWrite(1)
			err = owner.InviteToChannel(ctx, group, inputUser(dualID, dualHash))
			logText := prehistoryFault.LogText()
			prehistoryFault.Disarm()
			if err == nil {
				t.Fatalf("invite succeeded while the inbox write was failed\n%s", logText)
			}
			if prehistoryFault.Fails() == before || !strings.Contains(logText, "inbox down") {
				t.Fatalf("proxy did not fail the inbox write\n%s", logText)
			}
			var state int
			qerr := db.QueryRow(`SELECT state FROM channel_participants WHERE channel_id=? AND user_id=?`, groupID, dualID).Scan(&state)
			if qerr != nil && qerr != sql.ErrNoRows {
				t.Fatal(qerr)
			}
			if qerr == nil && state == 0 {
				var msgDate int32
				if derr := db.QueryRow(`SELECT date2 FROM channel_messages WHERE channel_id=? AND message_id=?`, groupID, oldID).Scan(&msgDate); derr != nil {
					t.Fatal(derr)
				}
				if _, uerr := db.Exec(`UPDATE channel_participants SET date2=? WHERE channel_id=? AND user_id=?`, msgDate, groupID, dualID); uerr != nil {
					t.Fatal(uerr)
				}
				res, _ := dual.Invoke(ctx, &mtproto.TLMessagesGetHistory{
					Constructor: mtproto.CRC32_messages_getHistory,
					Peer:        inputPeerChannel(groupID, groupHash),
					Limit:       50,
				})
				t.Fatalf("incomplete join left StateNormal and history=%v joinTop=%d\n%s", protoTexts(messagesOrEmpty(res)), joinTopOf(t, db, groupID, dualID), logText)
			}
			res, _ := dual.Invoke(ctx, &mtproto.TLMessagesGetHistory{
				Constructor: mtproto.CRC32_messages_getHistory,
				Peer:        inputPeerChannel(groupID, groupHash),
				Limit:       50,
			})
			if textIn(messagesOrEmpty(res), secret) {
				t.Fatalf("unfinished join returned the pre-join body: %v", protoTexts(messagesOrEmpty(res)))
			}
			if err = owner.InviteToChannel(ctx, group, inputUser(dualID, dualHash)); err != nil {
				t.Fatal(err)
			}
			var joined int
			if err = db.QueryRow(`SELECT state FROM channel_participants WHERE channel_id=? AND user_id=?`, groupID, dualID).Scan(&joined); err != nil || joined != 0 {
				t.Fatalf("retry did not finish the join: state=%d %v", joined, err)
			}
			if joinTopOf(t, db, groupID, dualID) < oldID {
				t.Fatal("retry did not record the join top")
			}
			how := alignJoinSecond(t, db, groupID, dualID, oldID, freshID)
			t.Logf("dual-failure retry same-second: %s", how)
			dualCh := waitChannel(t, ctx, dual, groupID)
			hist, _ := historyResult(t, dual, ctx, inputPeerChannel(groupID, dualCh.GetAccessHash()))
			if textIn(hist, secret) {
				t.Fatalf("retry still returned the pre-join body: %v", protoTexts(hist))
			}
		})
	}

	t.Run("promote_read_list", func(t *testing.T) {
		editChannelAdmin(t, ctx, owner, group, newbieID, newbieHash, &mtproto.ChatAdminRights{ChangeInfo: true})
		byID := channelMessages(t, newbie, ctx, newbieCh, oldID)
		if !textIn(byID, secret) {
			t.Fatalf("admin could not read the pre-join body: %v", protoTexts(byID))
		}
		ids, n := readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
		if containsID(ids, newbieID) {
			var raw []byte
			_ = db.QueryRow(`SELECT data FROM channel_feature WHERE channel_id=?`, groupID).Scan(&raw)
			t.Fatalf("promotion listed the admin before readHistory: count=%d ids=%v feature=%s", n, ids, raw)
		}
		if err = newbie.ReadChannelHistory(ctx, newbieCh, freshID); err != nil {
			t.Fatal(err)
		}
		ids, n = readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
		if !containsID(ids, newbieID) {
			t.Fatalf("readHistory after promotion did not list the admin: count=%d ids=%v", n, ids)
		}
		if prehistoryFault == nil {
			return
		}
		before := prehistoryFault.Fails()
		prehistoryFault.ArmFeatureRead()
		obj, rerr := invokeReadParticipants(owner, ownerPeer, oldID)
		prehistoryFault.Disarm()
		if rerr == nil || containsID(readerIDsOf(obj), newbieID) {
			t.Fatalf("getMessageReadParticipants returned a list after promotion when channel_feature could not be read: %v ids=%v\n%s", rerr, readerIDsOf(obj), prehistoryFault.LogText())
		}
		if prehistoryFault.Fails() == before {
			t.Fatalf("promoted read list did not read channel_feature\n%s", prehistoryFault.LogText())
		}
		ids, n = readParticipantIDs(t, owner, ctx, ownerPeer, oldID)
		if !containsID(ids, newbieID) {
			t.Fatalf("feature read failure wiped the admin read list: count=%d ids=%v", n, ids)
		}
	})

}

func openPrehistoryDB(t *testing.T) *sql.DB {
	t.Helper()
	candidates := []string{}
	if v := os.Getenv("TELLOMI_E2E_MYSQL_DSN"); v != "" {
		candidates = append(candidates, v)
	} else {
		candidates = append(candidates,
			"teamgram:teamgram@tcp(127.0.0.1:3306)/teamgram?parseTime=true&charset=utf8mb4&timeout=3s",
			"root:root@tcp(127.0.0.1:3306)/teamgram?parseTime=true&charset=utf8mb4&timeout=3s",
		)
	}
	var last error
	for _, dsn := range candidates {
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			last = err
			continue
		}
		if err = db.Ping(); err != nil {
			_ = db.Close()
			last = err
			continue
		}
		t.Cleanup(func() { _ = db.Close() })
		return db
	}
	t.Fatalf("isolated channel database: %v", last)
	return nil
}

func joinTopOf(t *testing.T, db *sql.DB, channelID, userID int64) int32 {
	t.Helper()
	var raw []byte
	if err := db.QueryRow(`SELECT data FROM channel_feature WHERE channel_id=?`, channelID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var feat struct {
		HiddenPrehistory bool             `json:"HiddenPrehistory"`
		JoinTop          map[string]int32 `json:"JoinTop"`
	}
	if err := json.Unmarshal(raw, &feat); err != nil {
		t.Fatal(err)
	}
	if !feat.HiddenPrehistory {
		t.Fatal("channel_feature does not record hidden prehistory")
	}
	return feat.JoinTop[strconv.FormatInt(userID, 10)]
}

func readInboxOf(t *testing.T, db *sql.DB, channelID, userID int64) int32 {
	t.Helper()
	var inbox int32
	err := db.QueryRow(`SELECT read_inbox_max_id FROM channel_read_state WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&inbox)
	if err != nil {
		t.Fatal(err)
	}
	return inbox
}

// alignJoinSecond makes the secret, the post-join message, and the new
// member share one unix second when the RPCs crossed a boundary. The read
// that follows is still messages.getHistory. A recorded join top is what
// hides the earlier id; the equal dates stop the older date-only rule from
// hiding it on their own.
func alignJoinSecond(t *testing.T, db *sql.DB, channelID, userID int64, oldID, freshID int32) string {
	t.Helper()
	var msgDate, joinDate, freshDate int32
	if err := db.QueryRow(`SELECT date2 FROM channel_messages WHERE channel_id=? AND message_id=?`, channelID, oldID).Scan(&msgDate); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT date2 FROM channel_messages WHERE channel_id=? AND message_id=?`, channelID, freshID).Scan(&freshDate); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT date2 FROM channel_participants WHERE channel_id=? AND user_id=?`, channelID, userID).Scan(&joinDate); err != nil {
		t.Fatal(err)
	}
	if msgDate == joinDate && freshDate == joinDate {
		return "rpc timestamps already share one second"
	}
	if _, err := db.Exec(`UPDATE channel_participants SET date2=? WHERE channel_id=? AND user_id=?`, msgDate, channelID, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE channel_messages SET date2=? WHERE channel_id=? AND message_id=?`, msgDate, channelID, freshID); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("isolated db set participant and post-join date2 from %d/%d to secret date2 %d", joinDate, freshDate, msgDate)
}

func historyResult(t *testing.T, c *Client, ctx interface{ Done() <-chan struct{} }, peer *mtproto.InputPeer) ([]*mtproto.Message, int32) {
	t.Helper()
	return historyResultCtx(t, c, peer)
}

func historyResultCtx(t *testing.T, c *Client, peer *mtproto.InputPeer) ([]*mtproto.Message, int32) {
	t.Helper()
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	res, err := c.Invoke(ctx, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		Limit:       50,
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, ok := messagesOf(res)
	if !ok {
		t.Fatalf("getHistory %T", res)
	}
	box := messagesOfResult(t, res)
	return msgs, box.GetCount()
}

func historyMessages(t *testing.T, c *Client, ctx interface{ Done() <-chan struct{} }, peer *mtproto.InputPeer) []*mtproto.Message {
	t.Helper()
	msgs, _ := historyResultCtx(t, c, peer)
	return msgs
}

func channelMessages(t *testing.T, c *Client, ctx interface{ Done() <-chan struct{} }, channel *mtproto.InputChannel, ids ...int32) []*mtproto.Message {
	t.Helper()
	callCtx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	res, err := c.Invoke(callCtx, &mtproto.TLChannelsGetMessages{
		Constructor:           mtproto.CRC32_channels_getMessages_ad8c9a23,
		Channel:               channel,
		Id_VECTORINPUTMESSAGE: inputMessageIDs(ids...),
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, ok := messagesOf(res)
	if !ok {
		t.Fatalf("channels.getMessages %T", res)
	}
	return msgs
}

func searchMessages(t *testing.T, c *Client, ctx interface{ Done() <-chan struct{} }, peer *mtproto.InputPeer, q string, filter *mtproto.MessagesFilter) []*mtproto.Message {
	t.Helper()
	callCtx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	res, err := c.Invoke(callCtx, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_a0fda762,
		Peer:        peer,
		Q:           q,
		Filter:      filter,
		Limit:       20,
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, ok := messagesOf(res)
	if !ok {
		t.Fatalf("messages.search %T", res)
	}
	return msgs
}

func invokeReadParticipants(c *Client, peer *mtproto.InputPeer, msgID int32) (mtproto.TLObject, error) {
	callCtx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	return c.Invoke(callCtx, &mtproto.TLMessagesGetMessageReadParticipants31C1C44F{
		Constructor: mtproto.CRC32_messages_getMessageReadParticipants31C1C44F,
		Peer:        peer,
		MsgId:       msgID,
	})
}

func readerIDsOf(obj mtproto.TLObject) []int64 {
	vec, ok := obj.(*tlVector)
	if !ok || vec == nil {
		return nil
	}
	ids := make([]int64, 0, len(vec.Readers))
	for _, r := range vec.Readers {
		if r.GetUserId() != 0 {
			ids = append(ids, r.GetUserId())
		}
	}
	return ids
}

func readParticipantIDs(t *testing.T, c *Client, ctx interface{ Done() <-chan struct{} }, peer *mtproto.InputPeer, msgID int32) ([]int64, int) {
	t.Helper()
	res, err := invokeReadParticipants(c, peer, msgID)
	if err != nil {
		t.Fatal(err)
	}
	vec, ok := res.(*tlVector)
	if !ok {
		t.Fatalf("getMessageReadParticipants %T", res)
	}
	if vec.Count > 0 && len(vec.Readers) != int(vec.Count) {
		t.Fatalf("read list vector count %d decoded %d (%s)", vec.Count, len(vec.Readers), vec.String())
	}
	ids := make([]int64, 0, len(vec.Readers))
	for _, r := range vec.Readers {
		if r.GetUserId() != 0 {
			ids = append(ids, r.GetUserId())
		}
	}
	return ids, len(ids)
}

func inputMessageIDs(ids ...int32) []*mtproto.InputMessage {
	out := make([]*mtproto.InputMessage, 0, len(ids))
	for _, id := range ids {
		out = append(out, mtproto.MakeTLInputMessageID(&mtproto.InputMessage{Id: id}).To_InputMessage())
	}
	return out
}

func differenceMessages(obj mtproto.TLObject) []*mtproto.Message {
	switch diff := obj.(type) {
	case *mtproto.TLUpdatesChannelDifference:
		msgs := append([]*mtproto.Message{}, diff.GetNewMessages()...)
		for _, u := range diff.GetOtherUpdates() {
			if m := u.GetMessage_MESSAGE(); m != nil {
				msgs = append(msgs, m)
			}
		}
		return msgs
	case *mtproto.TLUpdatesChannelDifferenceTooLong:
		return diff.GetMessages()
	default:
		return nil
	}
}

func discussionOrMessages(obj mtproto.TLObject) []*mtproto.Message {
	switch v := obj.(type) {
	case *mtproto.TLMessagesDiscussionMessage:
		if v.GetData2() != nil {
			return v.GetData2().GetMessages()
		}
	case *mtproto.Messages_DiscussionMessage:
		return v.GetMessages()
	}
	return messagesOrEmpty(obj)
}

func searchCounterTotal(obj mtproto.TLObject) int32 {
	var n int32
	switch vec := obj.(type) {
	case *mtproto.Vector_Messages_SearchCounter:
		for _, row := range vec.GetDatas() {
			n += row.GetCount()
		}
	case *tlVector:
		for _, row := range vec.Counters {
			n += row.GetCount()
		}
	}
	return n
}

func messagesOrEmpty(obj mtproto.TLObject) []*mtproto.Message {
	msgs, ok := messagesOf(obj)
	if !ok {
		return nil
	}
	return msgs
}

func protoTexts(msgs []*mtproto.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.GetMessage() != "" {
			out = append(out, m.GetMessage())
		}
	}
	return out
}

func textIn(msgs []*mtproto.Message, text string) bool {
	for _, m := range msgs {
		if m.GetMessage() == text || strings.Contains(m.GetMessage(), text) {
			return true
		}
	}
	return false
}

func messageEmpty(msgs []*mtproto.Message, id int32) bool {
	for _, m := range msgs {
		if m.GetId() == id {
			return m.GetPredicateName() == mtproto.Predicate_messageEmpty
		}
	}
	return false
}
