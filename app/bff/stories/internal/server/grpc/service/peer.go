package service

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
)

func resolvePeer(self int64, peer *mtproto.InputPeer) (story.Owner, int64, error) {
	if peer == nil {
		return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
	}
	p := mtproto.FromInputPeer2(self, peer)
	switch p.PeerType {
	case mtproto.PEER_SELF:
		return story.Owner{Kind: story.KindUser, ID: self}, 0, nil
	case mtproto.PEER_USER:
		if p.PeerId == 0 {
			return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
		}
		return story.Owner{Kind: story.KindUser, ID: p.PeerId}, p.AccessHash, nil
	case mtproto.PEER_CHANNEL:
		if p.PeerId == 0 {
			return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
		}
		return story.Owner{Kind: story.KindChannel, ID: p.PeerId}, p.AccessHash, nil
	default:
		return story.Owner{}, 0, mtproto.ErrPeerIdInvalid
	}
}

func privacyRules(self int64, rules []*mtproto.InputPrivacyRule) ([]*mtproto.PrivacyRule, error) {
	if len(rules) == 0 {
		return []*mtproto.PrivacyRule{mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule()}, nil
	}
	out := make([]*mtproto.PrivacyRule, 0, len(rules))
	for _, rule := range rules {
		if rule == nil {
			return nil, mtproto.ErrPrivacyValueInvalid
		}
		switch rule.PredicateName {
		case mtproto.Predicate_inputPrivacyValueAllowAll:
			out = append(out, mtproto.MakeTLPrivacyValueAllowAll(nil).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueAllowContacts:
			out = append(out, mtproto.MakeTLPrivacyValueAllowContacts(nil).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueAllowCloseFriends:
			out = append(out, mtproto.MakeTLPrivacyValueAllowCloseFriends(nil).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueAllowUsers:
			out = append(out, mtproto.MakeTLPrivacyValueAllowUsers(&mtproto.PrivacyRule{
				Users: mtproto.ToUserIdListByInput(self, rule.GetUsers()),
			}).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueDisallowUsers:
			out = append(out, mtproto.MakeTLPrivacyValueDisallowUsers(&mtproto.PrivacyRule{
				Users: mtproto.ToUserIdListByInput(self, rule.GetUsers()),
			}).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueDisallowAll:
			out = append(out, mtproto.MakeTLPrivacyValueDisallowAll(nil).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueDisallowContacts:
			out = append(out, mtproto.MakeTLPrivacyValueDisallowContacts(nil).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueAllowChatParticipants:
			out = append(out, mtproto.MakeTLPrivacyValueAllowChatParticipants(&mtproto.PrivacyRule{Chats: rule.GetChats()}).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueDisallowChatParticipants:
			out = append(out, mtproto.MakeTLPrivacyValueDisallowChatParticipants(&mtproto.PrivacyRule{Chats: rule.GetChats()}).To_PrivacyRule())
		case mtproto.Predicate_inputPrivacyValueAllowPremium:
			out = append(out, mtproto.MakeTLPrivacyValueAllowPremium(nil).To_PrivacyRule())
		default:
			return nil, mtproto.ErrPrivacyValueInvalid
		}
	}
	return out, nil
}

func idsOfItems(items []*mtproto.StoryItem) []int64 {
	var ids []int64
	for _, item := range items {
		if item.GetFromId().GetUserId() != 0 {
			ids = append(ids, item.GetFromId().GetUserId())
		}
		ids = append(ids, item.GetViews().GetRecentViewers()...)
	}
	return ids
}

func idsOfPeers(list []*mtproto.PeerStories) []int64 {
	var ids []int64
	for _, ps := range list {
		if ps.GetPeer().GetUserId() != 0 {
			ids = append(ids, ps.GetPeer().GetUserId())
		}
		ids = append(ids, idsOfItems(ps.GetStories())...)
	}
	return ids
}
