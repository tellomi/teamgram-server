package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/logx"
)

// stampChatDecor writes the chat theme or wallpaper from a service message
// onto the dialog row that was just created for userID. getFullUser reads
// those columns; the in-memory BFF map is not that source.
func (d *Dao) stampChatDecor(ctx context.Context, userID int64, peerType int32, peerID int64, message *mtproto.Message) {
	if d == nil || d.DialogClient == nil || message == nil || userID == 0 || peerID == 0 {
		return
	}
	action := message.GetAction()
	if action == nil {
		return
	}
	switch action.GetPredicateName() {
	case mtproto.Predicate_messageActionSetChatTheme:
		emoticon := action.GetEmoticon()
		if emoticon == "" && action.GetTheme() != nil {
			emoticon = action.GetTheme().GetEmoticon()
		}
		if _, err := d.DialogClient.DialogSetChatTheme(ctx, &dialog.TLDialogSetChatTheme{
			UserId:        userID,
			PeerType:      peerType,
			PeerId:        peerID,
			ThemeEmoticon: emoticon,
		}); err != nil {
			logx.WithContext(ctx).Errorf("stamp chat theme: %v", err)
		}
	case mtproto.Predicate_messageActionSetChatWallPaper:
		// Out is set on the sender's copy and cleared on the inbox copy.
		// FromId stays the user who called setChatWallPaper.
		ownerIsSender := message.GetOut() || message.GetFromId().GetUserId() == userID
		// A one-sided wallpaper is only the caller's. for_both is what the
		// other user actually receives.
		if !ownerIsSender && !action.GetForBoth() {
			return
		}
		id := int64(0)
		if paper := action.GetWallpaper(); paper != nil {
			id = paper.GetId()
		}
		if _, err := d.DialogClient.DialogSetChatWallpaper(ctx, &dialog.TLDialogSetChatWallpaper{
			UserId:      userID,
			PeerType:    peerType,
			PeerId:      peerID,
			WallpaperId: id,
			// True only when the other user installed this wallpaper for both.
			WallpaperOverridden: action.GetForBoth() && !ownerIsSender,
		}); err != nil {
			logx.WithContext(ctx).Errorf("stamp chat wallpaper: %v", err)
		}
	}
}
