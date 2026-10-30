// Package promo serves help.getPromoData and help.hidePromoData.
//
// This server has no PSA or MTProxy sponsor, so help.getPromoData returns
// help.promoDataEmpty. help.hidePromoData remembers the peer. A later
// announcement for that peer stays hidden.
//
// https://core.telegram.org/method/help.getPromoData
// https://core.telegram.org/method/help.hidePromoData
// https://core.telegram.org/constructor/help.promoDataEmpty
// https://core.telegram.org/constructor/help.promoData
// https://core.telegram.org/api/config#suggestions
package promo

import (
	"fmt"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// refillAfter is how long clients should wait before calling help.getPromoData
// again. core.telegram.org describes expires as seconds until the next fetch.
// Telegram Desktop (f985723b552b, the coverage snapshot) stores the integer as
// a unix deadline and waits until then, capped at one hour. A deadline of
// now+refillAfter matches that client. iOS ignores expires and polls itself.
const refillAfter = int32(60 * 60)

// Announcement is an optional PSA or MTProxy sponsor. Production leaves it unset.
type Announcement struct {
	Peer       *mtproto.Peer
	Proxy      bool
	PSAType    string
	PSAMessage string
}

// Empty is help.promoDataEmpty, the documented result when nothing is pinned.
func Empty() *mtproto.Help_PromoData {
	return mtproto.MakeTLHelpPromoDataEmpty(&mtproto.Help_PromoData{
		Expires: int32(time.Now().Unix()) + refillAfter,
	}).To_Help_PromoData()
}

func payload(a *Announcement) *mtproto.Help_PromoData {
	data := &mtproto.Help_PromoData{
		Expires:              int32(time.Now().Unix()) + refillAfter,
		Peer:                 a.Peer,
		Proxy:                a.Proxy,
		PendingSuggestions:   []string{},
		DismissedSuggestions: []string{},
		Chats:                []*mtproto.Chat{},
		Users:                []*mtproto.User{},
	}
	if a.PSAType != "" {
		data.PsaType = wrapperspb.String(a.PSAType)
	}
	if a.PSAMessage != "" {
		data.PsaMessage = wrapperspb.String(a.PSAMessage)
	}
	return mtproto.MakeTLHelpPromoData(data).To_Help_PromoData()
}

func inputKey(self int64, peer *mtproto.InputPeer) (string, error) {
	if peer == nil || peer.GetPredicateName() == "" {
		return "", mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, peer)
	switch p.PeerType {
	case mtproto.PEER_SELF, mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		if p.PeerType != mtproto.PEER_SELF && p.PeerId == 0 {
			return "", mtproto.ErrPeerIdInvalid
		}
		return fmt.Sprintf("%d:%d", p.PeerType, p.PeerId), nil
	default:
		return "", mtproto.ErrPeerIdInvalid
	}
}

func announcementKey(peer *mtproto.Peer) string {
	if peer == nil {
		return ""
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		if peer.GetUserId() == 0 {
			return ""
		}
		return fmt.Sprintf("%d:%d", mtproto.PEER_USER, peer.GetUserId())
	case mtproto.Predicate_peerChat:
		if peer.GetChatId() == 0 {
			return ""
		}
		return fmt.Sprintf("%d:%d", mtproto.PEER_CHAT, peer.GetChatId())
	case mtproto.Predicate_peerChannel:
		if peer.GetChannelId() == 0 {
			return ""
		}
		return fmt.Sprintf("%d:%d", mtproto.PEER_CHANNEL, peer.GetChannelId())
	default:
		return ""
	}
}
