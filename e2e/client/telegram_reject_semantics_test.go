package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestTelegramRejectSemantics covers ISSUE-S-EPHEMERAL-STUB-001 (row 348),
// ISSUE-S-CONF-NONCONF-001 (row 349), and ISSUE-S-WEBVIEW-CUSTOM-001 (row 350).
// Phones 9996610485–488, code 11111, layer 232.
func TestTelegramRejectSemantics(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	sender, _ := dialPhone(t, ctx, cfg, "9996610485", "EphUser")
	peer, peerSelf := dialPhone(t, ctx, cfg, "9996610486", "EphPeer")
	if _, err := sender.Invoke(ctx, &mtproto.TLEphemeralSendMessage{
		Constructor: mtproto.CRC32_ephemeral_sendMessage_68cbd09f,
		Peer:        inputPeerUser(peerSelf.ID, peerSelf.AccessHash),
		ReceiverId:  inputUser(peerSelf.ID, peerSelf.AccessHash),
		Message:     "only a bot may send this",
		RandomId:    time.Now().UnixNano(),
	}); err == nil {
		t.Fatal("user sent an ephemeral message")
	} else {
		wantRPC(t, err, 400, "USER_BOT_REQUIRED")
	}
	_ = peer

	host, hostSelf := dialPhone(t, ctx, cfg, "9996610487", "CallHost")
	guest, guestSelf := dialPhone(t, ctx, cfg, "9996610488", "CallGuest")
	guestID, guestHash, err := host.ImportPhone(ctx, "9996610488", "CallGuest")
	if err != nil {
		t.Fatal(err)
	}
	if guestID == 0 {
		guestID, guestHash = guestSelf.ID, guestSelf.AccessHash
	}
	_ = hostSelf
	chatID, err := host.CreateChat(ctx, fmt.Sprintf("conf-reject-%d", time.Now().Unix()), inputUser(guestID, guestHash))
	if err != nil {
		t.Fatal(err)
	}
	created, err := host.Invoke(ctx, &mtproto.TLPhoneCreateGroupCall{
		Constructor: mtproto.CRC32_phone_createGroupCall,
		Peer:        inputPeerChat(chatID),
		RandomId:    int32(time.Now().Unix() % 1_000_000_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := groupCallOf(created)
	if call == nil || call.GetConference() {
		t.Fatalf("group call %#v", created)
	}
	inCall := mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{
		Id: call.GetId(), AccessHash: call.GetAccessHash(),
	}).To_InputGroupCall()
	if _, err = host.Invoke(ctx, &mtproto.TLPhoneDeleteConferenceCallParticipants{
		Constructor: mtproto.CRC32_phone_deleteConferenceCallParticipants,
		Kick:        true,
		Call:        inCall,
		Ids:         []int64{guestID},
		Block:       []byte{1, 2, 3},
	}); err == nil {
		t.Fatal("deleteConferenceCallParticipants accepted a group call")
	} else {
		wantRPC(t, err, 400, "GROUPCALL_INVALID")
	}
	if _, err = host.Invoke(ctx, &mtproto.TLPhoneSendConferenceCallBroadcast{
		Constructor: mtproto.CRC32_phone_sendConferenceCallBroadcast,
		Call:        inCall,
		Block:       []byte{1, 2, 3},
	}); err == nil {
		t.Fatal("sendConferenceCallBroadcast accepted a group call")
	} else {
		wantRPC(t, err, 400, "GROUPCALL_INVALID")
	}
	_ = guest

	viewer, viewerSelf := dialPhone(t, ctx, cfg, "9996610489", "WebViewer")
	if _, err = viewer.Invoke(ctx, &mtproto.TLBotsInvokeWebViewCustomMethod{
		Constructor:  mtproto.CRC32_bots_invokeWebViewCustomMethod,
		Bot:          inputUser(viewerSelf.ID, viewerSelf.AccessHash),
		CustomMethod: "ping",
		Params:       mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"n":1}`}).To_DataJSON(),
	}); err == nil {
		t.Fatal("invokeWebViewCustomMethod accepted a user as a bot")
	} else {
		wantRPC(t, err, 400, "BOT_INVALID")
	}
	createdBot, err := viewer.Invoke(ctx, &mtproto.TLBotsCreateBot{
		Constructor: mtproto.CRC32_bots_createBot,
		Name:        "E2E Web",
		Username:    fmt.Sprintf("e2e%d_bot", time.Now().Unix()%1_000_000_000),
		ManagerId:   inputUser(6, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	bot, ok := createdBot.(*mtproto.TLUser)
	if !ok || !bot.GetBot() || bot.GetId() == 0 {
		t.Fatalf("createBot %#v", createdBot)
	}
	var botHash int64
	if bot.GetAccessHash() != nil {
		botHash = bot.GetAccessHash().GetValue()
	}
	if _, err = viewer.Invoke(ctx, &mtproto.TLBotsInvokeWebViewCustomMethod{
		Constructor:  mtproto.CRC32_bots_invokeWebViewCustomMethod,
		Bot:          inputUser(bot.GetId(), botHash),
		CustomMethod: "ping",
		Params:       mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"n":1}`}).To_DataJSON(),
	}); err == nil {
		t.Fatal("invokeWebViewCustomMethod returned success")
	} else {
		wantRPC(t, err, 400, "METHOD_INVALID")
	}
}

func groupCallOf(obj mtproto.TLObject) *mtproto.GroupCall {
	var ups []*mtproto.Update
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		ups = v.GetUpdates()
	case *mtproto.TLUpdatesCombined:
		ups = v.GetUpdates()
	}
	for _, u := range ups {
		if c := u.GetCall_GROUPCALL(); c != nil {
			return c
		}
	}
	return nil
}
