package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSearchDateMentionsAndSaved covers private-chat min/max date,
// basic-group my-mentions, and Saved Messages saved_peer_id.
// Phones 9996610760–9996610762, code 11111, layer 232.
func TestSearchDateMentionsAndSaved(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610760", "SearchA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610761", "SearchB")
	_, cSelf := dialPhone(t, ctx, cfg, "9996610762", "SearchC")

	token := fmt.Sprintf("lanesearch%d", time.Now().Unix())
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	peerForB := inputPeerUser(aSelf.ID, aSelf.AccessHash)
	if _, err := a.SendText(ctx, peerForA, token+" early", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Second)
	if _, err := a.SendText(ctx, peerForA, token+" late", nil); err != nil {
		t.Fatal(err)
	}
	hist, err := b.History(ctx, peerForB, 30)
	if err != nil {
		t.Fatal(err)
	}
	early := messageByText(hist, token+" early")
	late := messageByText(hist, token+" late")
	if early == nil || late == nil || late.GetDate() <= early.GetDate() {
		t.Fatalf("dates early=%v late=%v", early.GetDate(), late.GetDate())
	}
	mid := early.GetDate() + (late.GetDate()-early.GetDate())/2
	onlyEarly := searchTexts(t, b, peerForB, token, 0, mid)
	if !textListed(onlyEarly, token+" early") || textListed(onlyEarly, token+" late") {
		t.Fatalf("max_date %v", onlyEarly)
	}
	onlyLate := searchTexts(t, b, peerForB, token, mid, 0)
	if textListed(onlyLate, token+" early") || !textListed(onlyLate, token+" late") {
		t.Fatalf("min_date %v", onlyLate)
	}
	if len(searchTexts(t, b, peerForB, token, 0, early.GetDate()-30)) != 0 {
		t.Fatal("past window was not empty")
	}
	if len(searchTexts(t, b, peerForB, token, late.GetDate()+3600, 0)) != 0 {
		t.Fatal("future window was not empty")
	}

	chatID, err := a.CreateChat(ctx, "LaneSearch", inputUser(bSelf.ID, bSelf.AccessHash), inputUser(cSelf.ID, cSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	chat := inputPeerChat(chatID)
	mention := "ping"
	if _, err := a.SendText(ctx, chat, mention, []*mtproto.MessageEntity{
		mtproto.MakeTLInputMessageEntityMentionName(&mtproto.MessageEntity{
			Offset: 0,
			Length: int32(len(mention)),
			UserId_INPUTUSER: mtproto.MakeTLInputUser(&mtproto.InputUser{
				UserId:     bSelf.ID,
				AccessHash: bSelf.AccessHash,
			}).To_InputUser(),
		}).To_MessageEntity(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendText(ctx, chat, mention+" plain", nil); err != nil {
		t.Fatal(err)
	}
	mine := searchFilter(t, b, chat, "", mtproto.MakeTLInputMessagesFilterMyMentions(&mtproto.MessagesFilter{}).To_MessagesFilter())
	if !textListed(mine, mention) || textListed(mine, mention+" plain") {
		t.Fatalf("B mentions %v", mine)
	}
	sender := searchFilter(t, a, chat, "", mtproto.MakeTLInputMessagesFilterMyMentions(&mtproto.MessagesFilter{}).To_MessagesFilter())
	if textListed(sender, mention) {
		t.Fatalf("A should not see the mention of B: %v", sender)
	}

	if _, err := b.SendText(ctx, peerForB, token+" fromb", nil); err != nil {
		t.Fatal(err)
	}
	var fromBID int32
	if err := waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := a.History(ctx, peerForA, 30)
		if err != nil {
			return false, err
		}
		fromBID = idOfText(msgs, token+" fromb")
		return fromBID != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    peerForA,
		Id:          []int32{fromBID},
		RandomId:    []int64{randomID()},
		ToPeer:      inputPeerSelf(),
	}); err != nil {
		t.Fatal(err)
	}
	saved := searchSaved(t, a, token, inputPeerUser(bSelf.ID, bSelf.AccessHash))
	if !textListed(saved, token+" fromb") {
		t.Fatalf("saved peer %v", saved)
	}
}

func searchTexts(t *testing.T, c *Client, peer *mtproto.InputPeer, q string, minDate, maxDate int32) []string {
	t.Helper()
	return messageTexts(messagesOfResult(t, invokeOK(t, c, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_29ee847a,
		Peer:        peer,
		Q:           q,
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter(),
		MinDate:     minDate,
		MaxDate:     maxDate,
		Limit:       20,
	})))
}

func searchFilter(t *testing.T, c *Client, peer *mtproto.InputPeer, q string, filter *mtproto.MessagesFilter) []string {
	t.Helper()
	return messageTexts(messagesOfResult(t, invokeOK(t, c, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_29ee847a,
		Peer:        peer,
		Q:           q,
		Filter:      filter,
		Limit:       20,
	})))
}

func searchSaved(t *testing.T, c *Client, q string, saved *mtproto.InputPeer) []string {
	t.Helper()
	return messageTexts(messagesOfResult(t, invokeOK(t, c, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_29ee847a,
		Peer:        inputPeerSelf(),
		Q:           q,
		SavedPeerId: saved,
		Filter:      mtproto.MakeTLInputMessagesFilterEmpty(&mtproto.MessagesFilter{}).To_MessagesFilter(),
		Limit:       20,
	})))
}

func messageByText(msgs []*mtproto.Message, text string) *mtproto.Message {
	for _, m := range msgs {
		if m != nil && m.GetMessage() == text {
			return m
		}
	}
	return nil
}

func textListed(texts []string, want string) bool {
	for _, text := range texts {
		if text == want {
			return true
		}
	}
	return false
}
