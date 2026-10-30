package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEmptyDialogFiltersEncode(t *testing.T) {
	got := messagesDialogFilters(nil, false)
	if got.GetPredicateName() != mtproto.Predicate_messages_dialogFilters {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if len(raw) < 12 {
		t.Fatalf("short encoding %d", len(raw))
	}
	ctor := int32(raw[0]) | int32(raw[1])<<8 | int32(raw[2])<<16 | int32(raw[3])<<24
	if uint32(ctor) != 0x2ad93719 {
		t.Fatalf("constructor %x", uint32(ctor))
	}
	flags := int32(raw[4]) | int32(raw[5])<<8 | int32(raw[6])<<16 | int32(raw[7])<<24
	if flags != 0 {
		t.Fatalf("flags %x", flags)
	}

	tagged := messagesDialogFilters([]*mtproto.DialogFilter{}, true)
	buf = mtproto.NewEncodeBuf(128)
	if err := tagged.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	raw = buf.GetBuf()
	flags = int32(raw[4]) | int32(raw[5])<<8 | int32(raw[6])<<16 | int32(raw[7])<<24
	if flags != 1 {
		t.Fatalf("tags flags %x", flags)
	}
}

func TestSuggestedDialogFiltersEncode(t *testing.T) {
	got := suggestedDialogFilters()
	if len(got.GetDatas()) != 5 {
		t.Fatalf("suggestions %d", len(got.GetDatas()))
	}
	buf := mtproto.NewEncodeBuf(1024)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if len(buf.GetBuf()) < 8 {
		t.Fatal("empty encoding")
	}
}

func TestUpdateDialogFilterValidation(t *testing.T) {
	if err := validateDialogFilter(0, nil); err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("id 0: %v", err)
	}
	if err := validateDialogFilter(2, nil); err != nil {
		t.Fatalf("delete: %v", err)
	}
	empty := &mtproto.DialogFilter{}
	if err := validateDialogFilter(2, empty); err != mtproto.ErrFilterTitleEmpty {
		t.Fatalf("empty title: %v", err)
	}
	long := titled("this title is far too long")
	if err := validateDialogFilter(2, long); err != mtproto.ErrMessageTooLong {
		t.Fatalf("long title: %v", err)
	}
	noInclude := titled("Unread")
	if err := validateDialogFilter(2, noInclude); err != mtproto.ErrFilterIncludeEmpty {
		t.Fatalf("include: %v", err)
	}
	ok := titled("Unread")
	ok.Contacts = true
	ok.ExcludeRead = true
	if err := validateDialogFilter(2, ok); err != nil {
		t.Fatalf("flags: %v", err)
	}
	if ok.GetId() != 2 {
		t.Fatalf("id %d", ok.GetId())
	}
	mismatch := titled("Unread")
	mismatch.Id = 3
	mismatch.ExcludeRead = true
	if err := validateDialogFilter(2, mismatch); err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("mismatch: %v", err)
	}
	chatlist := titled("Shared")
	chatlist.PredicateName = mtproto.Predicate_dialogFilterChatlist
	chatlist.ExcludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 5}).To_InputPeer()}
	chatlist.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 6}).To_InputPeer()}
	if err := validateDialogFilter(4, chatlist); err != mtproto.ErrChatlistExcludeInvalid {
		t.Fatalf("chatlist exclude: %v", err)
	}
	badPeer := titled("People")
	badPeer.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{}).To_InputPeer()}
	if err := validateDialogFilter(4, badPeer); err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("peer: %v", err)
	}
	badChat := titled("People")
	badChat.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{}).To_InputPeer()}
	if err := validateDialogFilter(4, badChat); err != mtproto.ErrChatIdInvalid {
		t.Fatalf("chat: %v", err)
	}
	badMsg := titled("People")
	badMsg.IncludePeers = []*mtproto.InputPeer{mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{
		Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 9}).To_InputPeer(),
	}).To_InputPeer()}
	if err := validateDialogFilter(4, badMsg); err != mtproto.ErrMsgIdInvalid {
		t.Fatalf("msg: %v", err)
	}
}

func TestEmptyOrderSucceedsWithoutStorage(t *testing.T) {
	c := &FoldersCore{}
	got, err := c.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{})
	if err != nil || !mtproto.FromBool(got) {
		t.Fatalf("empty order: %v %v", got, err)
	}
	_, err = c.MessagesUpdateDialogFiltersOrder(&mtproto.TLMessagesUpdateDialogFiltersOrder{Order: []int32{0, -1}})
	if err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("negative id: %v", err)
	}
}

func TestApplyColorPolicy(t *testing.T) {
	next := &mtproto.DialogFilter{Color: wrapperspb.Int32(2)}
	prev := &mtproto.DialogFilter{Color: wrapperspb.Int32(1)}
	applyColorPolicy(next, prev, false)
	if next.GetColor().GetValue() != 1 {
		t.Fatalf("kept %d", next.GetColor().GetValue())
	}
	next.Color = wrapperspb.Int32(9)
	applyColorPolicy(next, nil, true)
	if next.GetColor() != nil {
		t.Fatalf("out of range color kept")
	}
}

func titled(text string) *mtproto.DialogFilter {
	return mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
		Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text:     text,
			Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities(),
	}).To_DialogFilter()
}
