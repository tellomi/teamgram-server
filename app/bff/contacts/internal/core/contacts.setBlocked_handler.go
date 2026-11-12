// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/book"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsSetBlocked
// contacts.setBlocked#94c65c76 flags:# my_stories_from:flags.0?true id:Vector<InputPeer> limit:int = Bool;
// https://core.telegram.org/method/contacts.setBlocked
// https://core.telegram.org/api/block
//
// Replaces one whole blocklist. my_stories_from selects the story blocklist;
// otherwise the main blocklist is replaced. id is the full new list. limit is
// the length the client loaded; the stored list is id.
func (c *ContactsCore) ContactsSetBlocked(in *mtproto.TLContactsSetBlocked) (*mtproto.Bool, error) {
	if err := c.requireSelf(); err != nil {
		return nil, err
	}
	peers := make([]*mtproto.PeerUtil, 0, len(in.GetId()))
	ids := make([]int64, 0, len(in.GetId()))
	seen := map[int64]bool{}
	for _, raw := range in.GetId() {
		peer, err := inputPeerUser(c.MD.UserId, raw)
		if err != nil {
			c.Logger.Errorf("contacts.setBlocked - error: %v", err)
			return nil, err
		}
		if seen[peer.PeerId] {
			continue
		}
		seen[peer.PeerId] = true
		peers = append(peers, peer)
		ids = append(ids, peer.PeerId)
	}
	if err := c.ensureUsers(ids); err != nil {
		c.Logger.Errorf("contacts.setBlocked - error: %v", err)
		return nil, err
	}
	if in.GetMyStoriesFrom() {
		return c.replaceStoryBlocks(peers)
	}
	return c.replaceMainBlocks(peers)
}

func (c *ContactsCore) replaceStoryBlocks(peers []*mtproto.PeerUtil) (*mtproto.Bool, error) {
	want := make([]book.Peer, 0, len(peers))
	for _, peer := range peers {
		want = append(want, book.Peer{Type: mtproto.PEER_USER, ID: peer.PeerId})
	}
	added, removed, err := c.relBook().ReplaceStories(c.ctx, c.MD.UserId, want, nowUnix())
	if err != nil {
		c.Logger.Errorf("contacts.setBlocked - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	for _, peer := range added {
		c.pushStory(peer, true)
	}
	for _, peer := range removed {
		c.pushStory(peer, false)
	}
	return mtproto.BoolTrue, nil
}

func (c *ContactsCore) replaceMainBlocks(peers []*mtproto.PeerUtil) (*mtproto.Bool, error) {
	current, err := c.svcCtx.Dao.UserGetBlockedList(c.ctx, &userpb.TLUserGetBlockedList{
		UserId: c.MD.UserId,
		Offset: 0,
		Limit:  10000,
	})
	if err != nil {
		c.Logger.Errorf("contacts.setBlocked - error: %v", err)
		return nil, err
	}
	if current == nil {
		current = &userpb.Vector_PeerBlocked{}
	}
	have := map[int64]bool{}
	for _, row := range current.GetDatas() {
		peer := row.GetPeerId()
		if peer == nil || peer.GetPredicateName() != mtproto.Predicate_peerUser || peer.GetUserId() == 0 {
			continue
		}
		have[peer.GetUserId()] = true
	}
	want := map[int64]*mtproto.PeerUtil{}
	for _, peer := range peers {
		want[peer.PeerId] = peer
	}
	for id := range have {
		if _, ok := want[id]; ok {
			continue
		}
		_, _ = c.svcCtx.Dao.UserUnBlockPeer(c.ctx, &userpb.TLUserUnBlockPeer{
			UserId:   c.MD.UserId,
			PeerType: mtproto.PEER_USER,
			PeerId:   id,
		})
		c.pushBlocked(&mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: id}, false, false)
	}
	for id, peer := range want {
		if have[id] {
			continue
		}
		_, _ = c.svcCtx.Dao.UserBlockPeer(c.ctx, &userpb.TLUserBlockPeer{
			UserId:   c.MD.UserId,
			PeerType: mtproto.PEER_USER,
			PeerId:   id,
		})
		c.pushBlocked(peer, true, false)
	}
	changed := []int64{c.MD.UserId}
	for id := range have {
		changed = append(changed, id)
	}
	for id := range want {
		changed = append(changed, id)
	}
	statusbook.Shared().Forget(c.ctx, changed...)
	return mtproto.BoolTrue, nil
}
