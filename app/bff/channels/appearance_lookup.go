package channels_helper

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

func init() {
	channel.SetAppearanceLookup(channel.AppearanceLookup{
		EmojiDocument: emojiDocumentKnown,
		StickerSet:    stickerSetByID,
	})
}

func emojiDocumentKnown(id int64) bool {
	if id <= 0 {
		return false
	}
	docs := catalog.Must().Documents([]int64{id})
	if docs == nil || len(docs.GetDatas()) == 0 {
		return false
	}
	return docs.GetDatas()[0].GetPredicateName() == mtproto.Predicate_document
}

func stickerSetByID(id int64) *mtproto.StickerSet {
	if id == 0 {
		return nil
	}
	set, err := catalog.Must().GetSet(0, mtproto.MakeTLInputStickerSetID(&mtproto.InputStickerSet{Id: id}).To_InputStickerSet(), 0)
	if err != nil || set == nil || set.GetSet() == nil || set.GetSet().GetId() == 0 {
		return nil
	}
	return set.GetSet()
}
