package core

import (
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// inputMediaIsVoice reports a voice note. Round videos are video messages
// and are not covered by the voice-message privacy key.
func inputMediaIsVoice(in *mtproto.InputMedia) bool {
	if in == nil {
		return false
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputMediaUploadedDocument, mtproto.Predicate_inputMediaDocument:
	default:
		return false
	}
	for _, attr := range in.GetAttributes() {
		if attr != nil && attr.GetPredicateName() == mtproto.Predicate_documentAttributeAudio && attr.GetVoice() {
			return true
		}
	}
	return false
}

// voiceForbidden is VOICE_MESSAGES_FORBIDDEN when the peer's voice-message
// privacy does not allow the sender.
func (c *MessagesCore) voiceForbidden(peer *mtproto.PeerUtil) error {
	if c == nil || c.MD == nil || peer == nil || peer.PeerType != mtproto.PEER_USER || peer.PeerId == 0 || peer.PeerId == c.MD.UserId {
		return nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	ok, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
		UserId:  peer.PeerId,
		KeyType: mtproto.VOICE_MESSAGES,
		PeerId:  c.MD.UserId,
	})
	if err != nil || mtproto.FromBool(ok) {
		return nil
	}
	return mtproto.ErrVoiceMessagesForbidden
}
