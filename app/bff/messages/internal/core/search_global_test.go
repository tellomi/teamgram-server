package core

import (
	"testing"

	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/pkg/searchorder"
)

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

func TestMergeGlobalPrivateChannelAndGroup(t *testing.T) {
	hello := &mtproto.MessageBox{
		PeerType: mtproto.PEER_USER, PeerId: 7, MessageId: 4,
		SenderUserId: 7,
		Message:      &mtproto.Message{Id: 4, Date: 40, Message: "Hello"},
	}
	p019 := &mtproto.MessageBox{
		PeerType: mtproto.PEER_CHAT, PeerId: 3, MessageId: 26,
		SenderUserId: 1,
		Message:      &mtproto.Message{Id: 26, Date: 30, Message: "p019"},
	}
	chn := &mtproto.MessageBox{
		PeerType: mtproto.PEER_CHANNEL, PeerId: 9, MessageId: 19,
		SenderUserId: 1,
		Message:      &mtproto.Message{Id: 19, Date: 50, Message: "chn019"},
	}
	merged := mergeGlobalBoxes(&mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{hello, p019}}, []*mtproto.MessageBox{chn, chn})
	sortGlobalBoxes(merged)
	if len(merged.BoxList) != 3 || merged.BoxList[0].GetMessage().GetMessage() != "chn019" {
		t.Fatalf("order %#v", texts(merged))
	}
	kinds := map[int64]channelKind{9: {Broadcast: true}}
	users := filterGlobalBoxes(cloneBoxes(merged), globalQuery{UsersOnly: true}, kinds, 10)
	if len(users.BoxList) != 1 || users.BoxList[0].GetMessage().GetMessage() != "Hello" {
		t.Fatalf("users_only %#v", texts(users))
	}
	broad := filterGlobalBoxes(cloneBoxes(merged), globalQuery{BroadcastsOnly: true}, kinds, 10)
	if len(broad.BoxList) != 1 || broad.BoxList[0].GetMessage().GetMessage() != "chn019" {
		t.Fatalf("broadcasts_only %#v", texts(broad))
	}
	folder := map[string]bool{
		peerKey(mtproto.PEER_USER, 7):    true,
		peerKey(mtproto.PEER_CHAT, 3):    true,
		peerKey(mtproto.PEER_CHANNEL, 9): true,
	}
	page := filterGlobalPage(cloneBoxes(merged), globalCursor{}, folder, true, 20)
	if len(page.BoxList) != 3 {
		t.Fatalf("folder kept %#v", texts(page))
	}
	withoutChannel := map[string]bool{
		peerKey(mtproto.PEER_USER, 7): true,
		peerKey(mtproto.PEER_CHAT, 3): true,
	}
	dropped := filterGlobalPage(cloneBoxes(merged), globalCursor{}, withoutChannel, true, 20)
	if len(dropped.BoxList) != 2 {
		t.Fatalf("channel must be in the folder map, got %#v", texts(dropped))
	}
}

func TestGlobalPagesDoNotMixMessageIDSpaces(t *testing.T) {
	// Page 1 ends on a channel post whose id is 5. A private row with box id
	// 100 is older by date. Bounding user rows with user_message_box_id < 5
	// would drop it. Page 2 must still return it, and page 3 the next private.
	channelHit := globalBox(mtproto.PEER_CHANNEL, 9, 5, 300, "chn019")
	private := globalBox(mtproto.PEER_USER, 7, 100, 200, "Hello")
	olderPrivate := globalBox(mtproto.PEER_USER, 7, 4, 100, "p019")
	all := []*mtproto.MessageBox{channelHit, private, olderPrivate}

	page1 := pageGlobalSources(all, globalCursor{}, 1, 200)
	if len(page1.BoxList) != 1 || page1.BoxList[0].GetMessage().GetMessage() != "chn019" {
		t.Fatalf("page 1 %#v", texts(page1))
	}
	cur := cursorOf(page1.BoxList[0])
	if cur.Rate != 300 || cur.MsgID != 5 || cur.PeerType != mtproto.PEER_CHANNEL {
		t.Fatalf("cursor %#v", cur)
	}
	page2 := pageGlobalSources(all, cur, 1, 200)
	if len(page2.BoxList) != 1 || page2.BoxList[0].GetMessage().GetMessage() != "Hello" || page2.BoxList[0].GetMessageId() != 100 {
		t.Fatalf("page 2 %#v", texts(page2))
	}
	page3 := pageGlobalSources(all, cursorOf(page2.BoxList[0]), 1, 200)
	if len(page3.BoxList) != 1 || page3.BoxList[0].GetMessage().GetMessage() != "p019" {
		t.Fatalf("page 3 %#v", texts(page3))
	}
}

func TestGlobalPageReachesChannelPastFetchCap(t *testing.T) {
	var all []*mtproto.MessageBox
	for i := 0; i < 201; i++ {
		all = append(all, globalBox(mtproto.PEER_CHANNEL, 9, int32(i+1), int32(2000-i), "hit"))
	}
	private := globalBox(mtproto.PEER_USER, 7, 5000, 10, "Hello")
	all = append(all, private)

	first := pageGlobalSources(all, globalCursor{}, 200, 200)
	if len(first.BoxList) != 200 || first.BoxList[len(first.BoxList)-1].GetMessage().GetDate() != 2000-199 {
		t.Fatalf("window end date %d len %d", first.BoxList[len(first.BoxList)-1].GetMessage().GetDate(), len(first.BoxList))
	}
	next := pageGlobalSources(all, cursorOf(first.BoxList[len(first.BoxList)-1]), 1, 200)
	if len(next.BoxList) != 1 || next.BoxList[0].GetMessage().GetDate() != 2000-200 {
		t.Fatalf("201st %#v", texts(next))
	}
	// Walk until the private message, which is older than every channel post.
	cur := globalCursor{}
	var sawPrivate bool
	for i := 0; i < 30 && !sawPrivate; i++ {
		page := pageGlobalSources(all, cur, 20, 200)
		if len(page.BoxList) == 0 {
			break
		}
		for _, box := range page.BoxList {
			if box.GetMessage().GetMessage() == "Hello" {
				sawPrivate = true
			}
		}
		cur = cursorOf(page.BoxList[len(page.BoxList)-1])
	}
	if !sawPrivate {
		t.Fatal("private message behind 201 channel posts was not reachable")
	}
}

func globalBox(peerType int32, peerID int64, id, date int32, text string) *mtproto.MessageBox {
	return &mtproto.MessageBox{
		PeerType:  peerType,
		PeerId:    peerID,
		MessageId: id,
		Message:   &mtproto.Message{Id: id, Date: date, Message: text},
	}
}

func cursorOf(box *mtproto.MessageBox) globalCursor {
	return globalCursor{
		Active:   true,
		Rate:     box.GetMessage().GetDate(),
		PeerType: box.GetPeerType(),
		PeerID:   box.GetPeerId(),
		MsgID:    box.GetMessage().GetId(),
	}
}

// pageGlobalSources is the two SQL windows. Each source returns the next
// fetchCap rows strictly after the cursor, in searchorder. A channel id is
// not compared with a user box id. filterGlobalPage is the same tie-break.
func pageGlobalSources(all []*mtproto.MessageBox, cur globalCursor, limit, fetchCap int32) *mtproto.MessageBoxList {
	var users, channels []*mtproto.MessageBox
	for _, box := range all {
		if cur.Active && !searchorder.Key(globalKey(box)).OlderThan(cur.key()) {
			continue
		}
		if box.GetPeerType() == mtproto.PEER_CHANNEL {
			channels = append(channels, box)
			continue
		}
		users = append(users, box)
	}
	users = newestCap(users, fetchCap)
	channels = newestCap(channels, fetchCap)
	merged := mergeGlobalBoxes(&mtproto.MessageBoxList{BoxList: users}, channels)
	sortGlobalBoxes(merged)
	return filterGlobalPage(merged, cur, nil, false, limit)
}

func newestCap(boxes []*mtproto.MessageBox, cap int32) []*mtproto.MessageBox {
	list := &mtproto.MessageBoxList{BoxList: append([]*mtproto.MessageBox(nil), boxes...)}
	sortGlobalBoxes(list)
	if cap > 0 && int32(len(list.BoxList)) > cap {
		list.BoxList = list.BoxList[:cap]
	}
	return list.BoxList
}

func TestEqualDateChannelStoreAndUserTuple(t *testing.T) {
	svc := channel.New(channel.NewMemoryStore())
	ctx := context.Background()
	created, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "eq", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	chID := created.Chats[0].Id
	const n = 30
	var users []*mtproto.MessageBox
	for i := int32(1); i <= n; i++ {
		users = append(users, globalBox(mtproto.PEER_USER, int64(i), 100+i, 50, "u"))
		if _, err := svc.Send(ctx, 1, chID, 0, []channel.Outgoing{{
			RandomID: int64(i),
			Message:  &mtproto.Message{Message: "c", Date: 50},
		}}); err != nil {
			t.Fatal(err)
		}
	}
	allCh, err := svc.SearchJoined(ctx, 1, "c", 100, searchorder.Key{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(allCh) != int(n) {
		t.Fatalf("channel hits %d", len(allCh))
	}
	var want []searchorder.Key
	for _, box := range users {
		want = append(want, globalKey(box))
	}
	for _, box := range allCh {
		want = append(want, searchorder.Key{Date: box.GetMessage().GetDate(), PeerType: mtproto.PEER_CHANNEL, PeerID: box.GetPeerId(), MsgID: box.GetMessageId()})
	}
	sortKeys(want)
	const pageSize = 4
	var got []searchorder.Key
	var cur searchorder.Key
	on := false
	for page := 0; page < 40; page++ {
		var pageRows []searchorder.Key
		for _, box := range users {
			key := globalKey(box)
			if on && !key.OlderThan(cur) {
				continue
			}
			pageRows = append(pageRows, key)
		}
		sortKeys(pageRows)
		if len(pageRows) > pageSize {
			pageRows = pageRows[:pageSize]
		}
		boxes, err := svc.SearchJoined(ctx, 1, "c", pageSize, cur, on)
		if err != nil {
			t.Fatal(err)
		}
		for _, box := range boxes {
			pageRows = append(pageRows, searchorder.Key{Date: box.GetMessage().GetDate(), PeerType: mtproto.PEER_CHANNEL, PeerID: box.GetPeerId(), MsgID: box.GetMessageId()})
		}
		sortKeys(pageRows)
		if len(pageRows) > pageSize {
			pageRows = pageRows[:pageSize]
		}
		if len(pageRows) == 0 {
			break
		}
		if len(pageRows) == pageSize {
			packed := packGlobalResult(&mtproto.Messages_Messages{Messages: keysAsMessages(pageRows)}, pageSize)
			if packed.GetPredicateName() != mtproto.Predicate_messages_messagesSlice || packed.GetNextRate().GetValue() != 50 || !packed.GetInexact() || packed.GetCount() != pageSize+1 {
				t.Fatalf("page %d slice pred %s rate %v count %d", page, packed.GetPredicateName(), packed.GetNextRate(), packed.GetCount())
			}
		}
		for _, key := range pageRows {
			for _, prev := range got {
				if prev == key {
					t.Fatalf("duplicate %#v", key)
				}
			}
			got = append(got, key)
		}
		cur = pageRows[len(pageRows)-1]
		on = true
		if len(pageRows) < pageSize {
			break
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d got %#v want %#v", i, got[i], want[i])
		}
	}
}

func sortKeys(keys []searchorder.Key) {
	for i := 1; i < len(keys); i++ {
		j := i
		for j > 0 && searchorder.Newer(keys[j], keys[j-1]) {
			keys[j], keys[j-1] = keys[j-1], keys[j]
			j--
		}
	}
}

func keysAsMessages(keys []searchorder.Key) []*mtproto.Message {
	out := make([]*mtproto.Message, len(keys))
	for i, key := range keys {
		out[i] = &mtproto.Message{Id: key.MsgID, Date: key.Date}
	}
	return out
}

func TestEqualDatePagesAcrossSources(t *testing.T) {
	var all []*mtproto.MessageBox
	for i := int32(1); i <= 40; i++ {
		all = append(all, globalBox(mtproto.PEER_USER, int64(i), 10+i, 50, "u"))
		all = append(all, globalBox(mtproto.PEER_CHANNEL, 1000+int64(i), i, 50, "c"))
	}
	want := append([]*mtproto.MessageBox(nil), all...)
	sortGlobalBoxes(&mtproto.MessageBoxList{BoxList: want})
	const pageSize int32 = 5
	const fetchCap int32 = 12
	var got []*mtproto.MessageBox
	cur := globalCursor{}
	var rates []int32
	for n := 0; n < 40; n++ {
		page := pageGlobalSources(all, cur, pageSize, fetchCap)
		if len(page.BoxList) == 0 {
			break
		}
		packed := packGlobalResult(&mtproto.Messages_Messages{Messages: messageTexts(page)}, pageSize)
		if int32(len(page.BoxList)) >= pageSize {
			if packed.GetPredicateName() != mtproto.Predicate_messages_messagesSlice || packed.GetNextRate().GetValue() != page.BoxList[len(page.BoxList)-1].GetMessage().GetDate() || !packed.GetInexact() || packed.GetCount() != pageSize+1 {
				t.Fatalf("slice %#v next %v count %d", packed.GetPredicateName(), packed.GetNextRate(), packed.GetCount())
			}
			rates = append(rates, packed.GetNextRate().GetValue())
		}
		got = append(got, page.BoxList...)
		cur = cursorOf(page.BoxList[len(page.BoxList)-1])
	}
	if len(got) != len(want) {
		t.Fatalf("got %d want %d", len(got), len(want))
	}
	for i := range want {
		if globalKey(got[i]) != globalKey(want[i]) {
			t.Fatalf("index %d got %#v want %#v", i, globalKey(got[i]), globalKey(want[i]))
		}
	}
	if len(rates) == 0 || rates[0] != 50 {
		t.Fatalf("next_rate %v", rates)
	}
}

func messageTexts(list *mtproto.MessageBoxList) []*mtproto.Message {
	var out []*mtproto.Message
	for _, box := range list.GetBoxList() {
		out = append(out, box.GetMessage())
	}
	return out
}

func TestAddJoinedChannelsRespectsArchive(t *testing.T) {
	folder := map[string]bool{}
	addJoinedChannels(folder, 0, []int64{9, 10}, map[int64]bool{10: true})
	if !folder[peerKey(mtproto.PEER_CHANNEL, 9)] || folder[peerKey(mtproto.PEER_CHANNEL, 10)] {
		t.Fatalf("main %#v", folder)
	}
	arch := map[string]bool{}
	addJoinedChannels(arch, 1, []int64{9, 10}, map[int64]bool{10: true})
	if arch[peerKey(mtproto.PEER_CHANNEL, 9)] || !arch[peerKey(mtproto.PEER_CHANNEL, 10)] {
		t.Fatalf("archive %#v", arch)
	}
}

func texts(list *mtproto.MessageBoxList) []string {
	var out []string
	for _, box := range list.GetBoxList() {
		out = append(out, box.GetMessage().GetMessage())
	}
	return out
}

func cloneBoxes(list *mtproto.MessageBoxList) *mtproto.MessageBoxList {
	out := &mtproto.MessageBoxList{BoxList: append([]*mtproto.MessageBox(nil), list.BoxList...)}
	return out
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
