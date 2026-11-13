package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDropPrehistoryBoxes(t *testing.T) {
	old := &mtproto.MessageBox{PeerType: mtproto.PEER_CHANNEL, PeerId: 9, Message: &mtproto.Message{Id: 1, Date: 10, Message: "old"}}
	fresh := &mtproto.MessageBox{PeerType: mtproto.PEER_CHANNEL, PeerId: 9, Message: &mtproto.Message{Id: 2, Date: 50, Message: "new"}}
	user := &mtproto.MessageBox{PeerType: mtproto.PEER_USER, PeerId: 3, Message: &mtproto.Message{Id: 3, Date: 10, Message: "dm"}}
	list := &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{old, fresh, user}}
	got := dropPrehistoryBoxes(list, func(channelID int64, date int32) bool {
		return channelID == 9 && date < 40
	})
	if len(got.BoxList) != 2 || got.BoxList[0].Message.Id != 2 || got.BoxList[1].PeerType != mtproto.PEER_USER {
		t.Fatalf("prehistory filter %+v", got.BoxList)
	}
}

func TestGlobalFilterEmptyQueryAndChineseURL(t *testing.T) {
	if globalFilterUseful(nil) || globalFilterUseful(mtproto.MakeTLInputMessagesFilterEmpty(nil).To_MessagesFilter()) {
		t.Fatal("empty filter should still reject an empty query")
	}
	photo := mtproto.MakeTLInputMessagesFilterPhotos(nil).To_MessagesFilter()
	url := mtproto.MakeTLInputMessagesFilterUrl(nil).To_MessagesFilter()
	voice := mtproto.MakeTLInputMessagesFilterVoice(nil).To_MessagesFilter()
	round := mtproto.MakeTLInputMessagesFilterRoundVoice(nil).To_MessagesFilter()
	if !globalFilterUseful(photo) || !globalFilterUseful(url) || !globalFilterUseful(voice) || !globalFilterUseful(round) {
		t.Fatal("client filters should allow an empty query")
	}
	link := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "看这里 https://example.com/中文",
	}).To_Message()
	plain := mtproto.MakeTLMessage(&mtproto.Message{Message: "中文没有链接"}).To_Message()
	if !messageMatchesGlobalFilter(link, url) || messageMatchesGlobalFilter(plain, url) {
		t.Fatal("url filter")
	}
	entity := mtproto.MakeTLMessage(&mtproto.Message{
		Message: "点这里",
		Entities: []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityTextUrl(&mtproto.MessageEntity{Url: "https://tellomi.cn"}).To_MessageEntity(),
		},
	}).To_Message()
	if !messageMatchesGlobalFilter(entity, url) {
		t.Fatal("text url entity")
	}
	pic := mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia(),
	}).To_Message()
	vid := mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				MimeType: "video/mp4",
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{W: 320, H: 240}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	if !messageMatchesGlobalFilter(pic, mtproto.MakeTLInputMessagesFilterPhotoVideo(nil).To_MessagesFilter()) ||
		!messageMatchesGlobalFilter(vid, mtproto.MakeTLInputMessagesFilterPhotoVideo(nil).To_MessagesFilter()) ||
		messageMatchesGlobalFilter(plain, mtproto.MakeTLInputMessagesFilterPhotoVideo(nil).To_MessagesFilter()) {
		t.Fatal("photo video")
	}
	spoken := mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	note := mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				MimeType: "video/mp4",
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{W: 240, H: 240, RoundMessage: true}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	if !messageMatchesGlobalFilter(spoken, voice) || messageMatchesGlobalFilter(note, voice) {
		t.Fatal("voice")
	}
	if !messageMatchesGlobalFilter(note, round) || messageMatchesGlobalFilter(spoken, round) {
		t.Fatal("round voice")
	}
}

func TestFilterGlobalBoxesByKindAndDate(t *testing.T) {
	user := &mtproto.MessageBox{PeerType: mtproto.PEER_USER, PeerId: 1, Message: &mtproto.Message{Date: 100, Message: "a"}}
	group := &mtproto.MessageBox{PeerType: mtproto.PEER_CHAT, PeerId: 2, Message: &mtproto.Message{Date: 200, Message: "b"}}
	news := &mtproto.MessageBox{PeerType: mtproto.PEER_CHANNEL, PeerId: 3, Message: &mtproto.Message{Date: 150, Message: "c"}}
	mega := &mtproto.MessageBox{PeerType: mtproto.PEER_CHANNEL, PeerId: 4, Message: &mtproto.Message{Date: 50, Message: "d"}}
	list := &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{user, group, news, mega}}
	kinds := map[int64]channelKind{
		3: {Broadcast: true},
		4: {Megagroup: true},
	}
	got := filterGlobalBoxes(list, globalQuery{BroadcastsOnly: true, MinDate: 100}, kinds, 10)
	if len(got.BoxList) != 1 || got.BoxList[0].PeerId != 3 {
		t.Fatalf("broadcasts %#v", got.BoxList)
	}

	list = &mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{user, group, news, mega}}
	got = filterGlobalBoxes(list, globalQuery{GroupsOnly: true, UsersOnly: true}, kinds, 1)
	if len(got.BoxList) != 1 || got.BoxList[0].PeerType == mtproto.PEER_CHANNEL {
		t.Fatalf("limit %#v", got.BoxList)
	}
}
