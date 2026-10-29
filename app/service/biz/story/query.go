package story

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

func (s *Service) filter(ctx context.Context, viewer int64, owner Owner, expired bool, pinnedOnly bool) ([]*Record, error) {
	recs, err := s.store.ListByOwner(ctx, owner)
	if err != nil {
		return nil, err
	}
	var out []*Record
	for _, rec := range recs {
		if pinnedOnly {
			if s.visible(ctx, viewer, rec, true, true) {
				out = append(out, rec)
			}
			continue
		}
		if expired {
			if rec.Deleted || rec.Item == nil || rec.active(s.now()) {
				continue
			}
			if s.privacyAllows(ctx, rec, viewer) {
				out = append(out, rec)
			}
			continue
		}
		if s.visible(ctx, viewer, rec, false, false) {
			out = append(out, rec)
		}
	}
	return out, nil
}

func (s *Service) GetPeerStories(ctx context.Context, viewer int64, owner Owner, accessHash int64) (*mtproto.Stories_PeerStories, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	recs, err := s.filter(ctx, viewer, owner, false, false)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLStoriesPeerStories(&mtproto.Stories_PeerStories{
		Stories: s.peerStories(ctx, viewer, owner, recs),
		Chats:   s.chats(ctx, viewer, owner),
		Users:   []*mtproto.User{},
	}).To_Stories_PeerStories(), nil
}

func (s *Service) GetByID(ctx context.Context, viewer int64, owner Owner, accessHash int64, ids []int32) (*mtproto.Stories_Stories, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	albums := s.albumIndex(ctx, owner)
	items := make([]*mtproto.StoryItem, 0, len(ids))
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		if rec == nil || rec.Deleted || !s.privacyAllows(ctx, rec, viewer) {
			items = append(items, s.deletedItem(id))
			continue
		}
		items = append(items, s.present(ctx, viewer, rec, albums))
	}
	top, _ := s.store.PinnedTop(ctx, owner)
	return mtproto.MakeTLStoriesStories(&mtproto.Stories_Stories{
		Count:       int32(len(items)),
		Stories:     items,
		PinnedToTop: top,
		Chats:       s.chats(ctx, viewer, owner),
		Users:       []*mtproto.User{},
	}).To_Stories_Stories(), nil
}

func (s *Service) GetPinned(ctx context.Context, viewer int64, owner Owner, accessHash int64, offset, limit int32) (*mtproto.Stories_Stories, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	recs, err := s.filter(ctx, viewer, owner, false, true)
	if err != nil {
		return nil, err
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].id() > recs[j].id() })
	limit = clampLimit(limit, 50, 100)
	var page []*Record
	for _, rec := range recs {
		if offset > 0 && rec.id() >= offset {
			continue
		}
		page = append(page, rec)
		if int32(len(page)) >= limit {
			break
		}
	}
	box := s.storiesBox(ctx, viewer, owner, page)
	box.Count = int32(len(recs))
	return box, nil
}

func (s *Service) GetArchive(ctx context.Context, viewer int64, owner Owner, accessHash int64, offset, limit int32) (*mtproto.Stories_Stories, error) {
	if !s.isManager(ctx, viewer, owner) {
		return nil, mtproto.ErrChatAdminRequired
	}
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	recs, err := s.filter(ctx, viewer, owner, true, false)
	if err != nil {
		return nil, err
	}
	sort.Slice(recs, func(i, j int) bool { return recs[i].id() > recs[j].id() })
	limit = clampLimit(limit, 50, 100)
	var page []*Record
	for _, rec := range recs {
		if offset > 0 && rec.id() >= offset {
			continue
		}
		page = append(page, rec)
		if int32(len(page)) >= limit {
			break
		}
	}
	box := s.storiesBox(ctx, viewer, owner, page)
	box.Count = int32(len(recs))
	return box, nil
}

type peerBlock struct {
	owner Owner
	recs  []*Record
}

func (s *Service) GetAll(ctx context.Context, viewer int64, hidden, next bool, state string) (*mtproto.Stories_AllStories, error) {
	settings, err := s.store.GetSettings(ctx, viewer)
	if err != nil {
		return nil, err
	}
	owners := []Owner{{Kind: KindUser, ID: viewer}}
	for _, id := range s.contactIDs(ctx, viewer) {
		owners = append(owners, Owner{Kind: KindUser, ID: id})
	}
	owners = append(owners, s.addressedOwners(ctx, viewer)...)
	if ids, err := channel.Shared().JoinedChannelIDs(ctx, viewer); err == nil {
		for _, id := range ids {
			owners = append(owners, Owner{Kind: KindChannel, ID: id})
		}
	}
	seen := map[Owner]bool{}
	var blocks []peerBlock
	var parts []string
	for _, owner := range owners {
		if seen[owner] || !owner.ok() {
			continue
		}
		seen[owner] = true
		// Self stays on the main tray. Everyone else moves to the hidden tray
		// when that peer is hidden, or when the viewer archived all user stories.
		onHidden := false
		if !(owner.Kind == KindUser && owner.ID == viewer) {
			onHidden = s.hidden(ctx, viewer, owner) || (settings.AllHidden && owner.Kind == KindUser)
		}
		if onHidden != hidden {
			continue
		}
		recs, err := s.filter(ctx, viewer, owner, false, false)
		if err != nil {
			return nil, err
		}
		if len(recs) == 0 {
			continue
		}
		blocks = append(blocks, peerBlock{owner: owner, recs: recs})
		parts = append(parts, owner.Peer().PredicateName+":"+strconv.FormatInt(owner.ID, 10)+":"+strconv.Itoa(len(recs))+":"+strconv.Itoa(int(recs[len(recs)-1].id())))
	}
	sum := hashState(hidden, parts)
	if state != "" && state == sum {
		return mtproto.MakeTLStoriesAllStoriesNotModified(&mtproto.Stories_AllStories{
			State:       sum,
			StealthMode: s.stealthMode(ctx, viewer),
		}).To_Stories_AllStories(), nil
	}
	_ = next
	peers := make([]*mtproto.PeerStories, 0, len(blocks))
	var chatOwners []Owner
	for _, b := range blocks {
		peers = append(peers, s.peerStories(ctx, viewer, b.owner, b.recs))
		chatOwners = append(chatOwners, b.owner)
	}
	return mtproto.MakeTLStoriesAllStories(&mtproto.Stories_AllStories{
		Count:       int32(len(peers)),
		State:       sum,
		PeerStories: peers,
		Chats:       s.chats(ctx, viewer, chatOwners...),
		Users:       []*mtproto.User{},
		StealthMode: s.stealthMode(ctx, viewer),
	}).To_Stories_AllStories(), nil
}

func (s *Service) addressedOwners(ctx context.Context, viewer int64) []Owner {
	all, err := s.store.ListAll(ctx)
	if err != nil {
		return nil
	}
	seen := map[Owner]bool{}
	var out []Owner
	for _, rec := range all {
		if seen[rec.Owner] || rec.Owner.Kind != KindUser || rec.Owner.ID == viewer || !rec.active(s.now()) {
			continue
		}
		if !addresses(rec, viewer) {
			continue
		}
		if s.privacyAllows(ctx, rec, viewer) {
			seen[rec.Owner] = true
			out = append(out, rec.Owner)
		}
	}
	return out
}

func addresses(rec *Record, viewer int64) bool {
	for _, r := range rec.Item.GetPrivacy() {
		switch r.GetPredicateName() {
		case mtproto.Predicate_privacyValueAllowUsers:
			if contains64(r.Users, viewer) {
				return true
			}
		case mtproto.Predicate_privacyValueAllowCloseFriends:
			return true
		}
	}
	return false
}

func (s *Service) PeerMaxIDs(ctx context.Context, viewer int64, owners []Owner) []*mtproto.RecentStory {
	out := make([]*mtproto.RecentStory, 0, len(owners))
	for _, owner := range owners {
		rs := &mtproto.RecentStory{}
		if err := s.canViewPeer(ctx, viewer, owner, 0); err == nil {
			recs, _ := s.filter(ctx, viewer, owner, false, false)
			var max int32
			var live bool
			for _, rec := range recs {
				if rec.id() >= max {
					max = rec.id()
					live = rec.Item.GetLive()
				}
			}
			if max > 0 {
				rs.Live = live
				rs.MaxId = &wrapperspb.Int32Value{Value: max}
			}
		}
		out = append(out, mtproto.MakeTLRecentStory(rs).To_RecentStory())
	}
	return out
}

func (s *Service) ChatsToSend(ctx context.Context, actor int64) []*mtproto.Chat {
	ids, err := channel.Shared().JoinedChannelIDs(ctx, actor)
	if err != nil {
		return []*mtproto.Chat{}
	}
	var postable []int64
	for _, id := range ids {
		_, post, _, _, err := channel.Shared().StoryPerm(ctx, actor, id, 0)
		if err == nil && post {
			postable = append(postable, id)
		}
	}
	if len(postable) == 0 {
		return []*mtproto.Chat{}
	}
	return channel.Shared().ChatsFor(ctx, actor, postable...)
}

func (s *Service) ToggleAllHidden(ctx context.Context, viewer int64, hidden bool) error {
	st, err := s.store.GetSettings(ctx, viewer)
	if err != nil {
		return err
	}
	st.AllHidden = hidden
	return s.store.SaveSettings(ctx, st)
}

func (s *Service) TogglePeerHidden(ctx context.Context, viewer int64, owner Owner, accessHash int64, hidden bool) error {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return err
	}
	rd, err := s.store.GetRead(ctx, viewer, owner)
	if err != nil {
		return err
	}
	if rd == nil {
		rd = &Read{UserID: viewer, Owner: owner}
	}
	rd.Hidden = hidden
	if err = s.store.SaveRead(ctx, rd); err != nil {
		return err
	}
	if owner.Kind == KindUser && owner.ID != viewer {
		s.mu.Lock()
		fn := s.hooks.SetStoriesHidden
		s.mu.Unlock()
		if fn != nil {
			fn(ctx, viewer, owner.ID, hidden)
		}
	}
	return nil
}

func (s *Service) Read(ctx context.Context, viewer, auth int64, owner Owner, accessHash int64, maxID int32) ([]int32, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	recs, err := s.filter(ctx, viewer, owner, false, false)
	if err != nil {
		return nil, err
	}
	var ids []int32
	for _, rec := range recs {
		if rec.id() <= maxID {
			ids = append(ids, rec.id())
		}
	}
	rd, err := s.store.GetRead(ctx, viewer, owner)
	if err != nil {
		return nil, err
	}
	if rd == nil {
		rd = &Read{UserID: viewer, Owner: owner}
	}
	if maxID > rd.MaxID {
		rd.MaxID = maxID
	}
	if err = s.store.SaveRead(ctx, rd); err != nil {
		return nil, err
	}
	s.push(ctx, viewer, []int64{auth}, mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdateReadStories(&mtproto.Update{
				Peer_PEER: owner.Peer(),
				MaxId:     rd.MaxID,
			}).To_Update(),
		},
		Users: []*mtproto.User{},
		Chats: s.chats(ctx, viewer, owner),
		Date:  s.now(),
	}).To_Updates())
	if ids == nil {
		ids = []int32{}
	}
	return ids, nil
}

func (s *Service) AllRead(ctx context.Context, viewer int64) (*mtproto.Updates, error) {
	reads, err := s.store.ListReads(ctx, viewer)
	if err != nil {
		return nil, err
	}
	var ups []*mtproto.Update
	var owners []Owner
	for _, rd := range reads {
		if rd.MaxID == 0 {
			continue
		}
		ups = append(ups, mtproto.MakeTLUpdateReadStories(&mtproto.Update{
			Peer_PEER: rd.Owner.Peer(),
			MaxId:     rd.MaxID,
		}).To_Update())
		owners = append(owners, rd.Owner)
	}
	if ups == nil {
		ups = []*mtproto.Update{}
	}
	return mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: ups,
		Users:   []*mtproto.User{},
		Chats:   s.chats(ctx, viewer, owners...),
		Date:    s.now(),
	}).To_Updates(), nil
}

func (s *Service) IncrementViews(ctx context.Context, viewer int64, owner Owner, accessHash int64, ids []int32) error {
	if len(ids) == 0 {
		return mtproto.ErrStoryIdEmpty
	}
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return err
	}
	st, _ := s.store.GetSettings(ctx, viewer)
	stealth := st != nil && st.StealthActive > s.now()
	now := s.now()
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return err
		}
		if rec == nil || rec.Deleted || !rec.active(now) || !s.privacyAllows(ctx, rec, viewer) {
			continue
		}
		if viewer == rec.Sender {
			continue
		}
		prev := s.findView(ctx, owner, id, viewer)
		if prev != nil {
			prev.Date = now
			if stealth {
				prev.Stealth = true
			}
			if err = s.store.SaveView(ctx, owner, id, *prev); err != nil {
				return err
			}
			continue
		}
		if err = s.store.SaveView(ctx, owner, id, View{UserID: viewer, Date: now, Stealth: stealth}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) findView(ctx context.Context, owner Owner, storyID int32, viewer int64) *View {
	views, _ := s.store.ListViews(ctx, owner, storyID)
	for _, v := range views {
		if v.UserID == viewer {
			cp := v
			return &cp
		}
	}
	return nil
}

// ViewsQuery is stories.getStoryViewsList.
type ViewsQuery struct {
	Actor          int64
	Owner          Owner
	AccessHash     int64
	ID             int32
	Offset         string
	Limit          int32
	Q              string
	JustContacts   bool
	ReactionsFirst bool
	ForwardsFirst  bool
	Reaction       string
}

func (s *Service) ViewsList(ctx context.Context, q ViewsQuery) (*mtproto.Stories_StoryViewsList, error) {
	if !s.isManager(ctx, q.Actor, q.Owner) {
		return nil, mtproto.ErrChatAdminRequired
	}
	rec, err := s.store.Get(ctx, q.Owner, q.ID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.Deleted {
		return nil, errStoryIDInvalid
	}
	views, err := s.store.ListViews(ctx, q.Owner, q.ID)
	if err != nil {
		return nil, err
	}
	var shown []View
	for _, v := range views {
		if v.Stealth {
			continue
		}
		if q.JustContacts && !s.isContact(ctx, privacySubject(rec), v.UserID) {
			continue
		}
		if q.Reaction != "" && v.Reaction != q.Reaction {
			continue
		}
		if q.Q != "" && !s.matchQuery(ctx, v.UserID, q.Q) {
			continue
		}
		shown = append(shown, v)
	}
	sort.SliceStable(shown, func(i, j int) bool {
		if q.ReactionsFirst && (shown[i].Reaction != "") != (shown[j].Reaction != "") {
			return shown[i].Reaction != ""
		}
		if shown[i].Date == shown[j].Date {
			return shown[i].UserID < shown[j].UserID
		}
		return shown[i].Date > shown[j].Date
	})
	_ = q.ForwardsFirst
	offset := 0
	if q.Offset != "" {
		offset, _ = strconv.Atoi(q.Offset)
	}
	if offset < 0 || offset > len(shown) {
		offset = 0
	}
	limit := int(clampLimit(q.Limit, 50, 100))
	end := offset + limit
	var next string
	if end < len(shown) {
		next = strconv.Itoa(end)
	} else {
		end = len(shown)
	}
	page := shown[offset:end]
	outViews := make([]*mtproto.StoryView, 0, len(page))
	for _, v := range page {
		sv := mtproto.MakeTLStoryView(&mtproto.StoryView{
			UserId: v.UserID,
			Date:   v.Date,
		}).To_StoryView()
		if v.Reaction != "" {
			sv.Reaction = mtproto.FromReaction(v.Reaction)
		}
		outViews = append(outViews, sv)
	}
	res := mtproto.MakeTLStoriesStoryViewsList(&mtproto.Stories_StoryViewsList{
		Count:          int32(len(shown)),
		ViewsCount:     int32(len(views)),
		ForwardsCount:  rec.Forwards,
		ReactionsCount: reactionCount(views),
		Views:          outViews,
		Chats:          []*mtproto.Chat{},
		Users:          []*mtproto.User{},
	}).To_Stories_StoryViewsList()
	if next != "" {
		res.NextOffset = &wrapperspb.StringValue{Value: next}
	}
	return res, nil
}

func reactionCount(views []View) int32 {
	var n int32
	for _, v := range views {
		if !v.Stealth && v.Reaction != "" {
			n++
		}
	}
	return n
}

func (s *Service) matchQuery(ctx context.Context, userID int64, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	if strings.Contains(strconv.FormatInt(userID, 10), q) {
		return true
	}
	s.mu.Lock()
	fn := s.hooks.MatchName
	s.mu.Unlock()
	return fn != nil && fn(ctx, userID, q)
}

func (s *Service) GetViews(ctx context.Context, viewer int64, owner Owner, accessHash int64, ids []int32) (*mtproto.Stories_StoryViews, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	views := make([]*mtproto.StoryViews, 0, len(ids))
	manager := s.isManager(ctx, viewer, owner)
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		if rec == nil || rec.Deleted || !s.privacyAllows(ctx, rec, viewer) {
			views = append(views, mtproto.MakeTLStoryViews(&mtproto.StoryViews{}).To_StoryViews())
			continue
		}
		views = append(views, s.viewSummary(ctx, viewer, rec, manager))
	}
	return mtproto.MakeTLStoriesStoryViews(&mtproto.Stories_StoryViews{
		Views: views,
		Users: []*mtproto.User{},
	}).To_Stories_StoryViews(), nil
}

func (s *Service) React(ctx context.Context, viewer, auth int64, owner Owner, accessHash int64, storyID int32, reaction *mtproto.Reaction) (*mtproto.Updates, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	rec, err := s.store.Get(ctx, owner, storyID)
	if err != nil {
		return nil, err
	}
	if rec == nil || rec.Deleted || !s.privacyAllows(ctx, rec, viewer) {
		return nil, errStoryIDInvalid
	}
	st, _ := s.store.GetSettings(ctx, viewer)
	stealth := st != nil && st.StealthActive > s.now()
	prev := s.findView(ctx, owner, storyID, viewer)
	view := View{UserID: viewer, Date: s.now(), Stealth: stealth}
	if prev != nil {
		view = *prev
		view.Date = s.now()
		if stealth {
			view.Stealth = true
		}
	}
	if reaction == nil || reaction.GetPredicateName() == mtproto.Predicate_reactionEmpty || reaction.GetPredicateName() == "" {
		view.Reaction = ""
	} else {
		view.Reaction = reaction.ToString()
	}
	if err = s.store.SaveView(ctx, owner, storyID, view); err != nil {
		return nil, err
	}
	up := s.storyUpdates(ctx, viewer, rec, false)
	s.push(ctx, viewer, []int64{auth}, up)
	if rec.Sender != viewer {
		s.push(ctx, rec.Sender, nil, s.storyUpdates(ctx, rec.Sender, rec, false))
	}
	return up, nil
}

func (s *Service) ReactionsList(ctx context.Context, q ViewsQuery) (*mtproto.Stories_StoryReactionsList, error) {
	list, err := s.ViewsList(ctx, q)
	if err != nil {
		return nil, err
	}
	reactions := make([]*mtproto.StoryReaction, 0, len(list.Views))
	for _, v := range list.Views {
		if v.GetReaction() == nil {
			continue
		}
		reactions = append(reactions, mtproto.MakeTLStoryReaction(&mtproto.StoryReaction{
			PeerId:   mtproto.MakePeerUser(v.UserId),
			Date:     v.Date,
			Reaction: v.Reaction,
		}).To_StoryReaction())
	}
	res := mtproto.MakeTLStoriesStoryReactionsList(&mtproto.Stories_StoryReactionsList{
		Count:     int32(len(reactions)),
		Reactions: reactions,
		Chats:     []*mtproto.Chat{},
		Users:     []*mtproto.User{},
	}).To_Stories_StoryReactionsList()
	if list.NextOffset != nil && int(list.Count) > len(reactions) {
		res.NextOffset = list.NextOffset
	}
	return res, nil
}

func (s *Service) ExportLink(ctx context.Context, viewer int64, owner Owner, accessHash int64, id int32) (string, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return "", err
	}
	rec, err := s.store.Get(ctx, owner, id)
	if err != nil {
		return "", err
	}
	if rec == nil || rec.Deleted || !s.privacyAllows(ctx, rec, viewer) {
		return "", errStoryIDInvalid
	}
	host := linkHost()
	idStr := strconv.FormatInt(int64(id), 10)
	if owner.Kind == KindChannel {
		if name := s.channelUsername(ctx, viewer, owner.ID); name != "" {
			return host + "/" + name + "/s/" + idStr, nil
		}
		return host + "/c/" + strconv.FormatInt(owner.ID, 10) + "/s/" + idStr, nil
	}
	s.mu.Lock()
	nameFn := s.hooks.Username
	s.mu.Unlock()
	if nameFn != nil {
		if name := nameFn(ctx, owner.ID); name != "" {
			return host + "/" + name + "/s/" + idStr, nil
		}
	}
	return host + "/s/" + strconv.FormatInt(owner.ID, 10) + "/" + idStr, nil
}

func (s *Service) channelUsername(ctx context.Context, viewer, id int64) string {
	chats := channel.Shared().ChatsFor(ctx, viewer, id)
	if len(chats) == 0 {
		return ""
	}
	return chats[0].GetUsername().GetValue()
}

func (s *Service) Report(ctx context.Context, viewer int64, owner Owner, accessHash int64, ids []int32, option []byte, message string) (*mtproto.ReportResult, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrStoryIdEmpty
	}
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	if len(option) == 0 {
		return mtproto.MakeTLReportResultChooseOption(&mtproto.ReportResult{
			Title: "Report story",
			Options: []*mtproto.MessageReportOption{
				mtproto.MakeTLMessageReportOption(&mtproto.MessageReportOption{Text: "Spam", Option: []byte("spam")}).To_MessageReportOption(),
				mtproto.MakeTLMessageReportOption(&mtproto.MessageReportOption{Text: "Violence", Option: []byte("violence")}).To_MessageReportOption(),
				mtproto.MakeTLMessageReportOption(&mtproto.MessageReportOption{Text: "Illegal content", Option: []byte("illegal")}).To_MessageReportOption(),
				mtproto.MakeTLMessageReportOption(&mtproto.MessageReportOption{Text: "Other", Option: []byte("other")}).To_MessageReportOption(),
			},
		}).To_ReportResult(), nil
	}
	if err := s.store.SaveReport(ctx, &Report{
		Reporter: viewer,
		Owner:    owner,
		StoryIDs: append([]int32(nil), ids...),
		Option:   string(option),
		Message:  message,
		Date:     s.now(),
	}); err != nil {
		return nil, err
	}
	return mtproto.MakeTLReportResultReported(&mtproto.ReportResult{}).To_ReportResult(), nil
}

func (s *Service) Stealth(ctx context.Context, viewer, auth int64, past, future bool) (*mtproto.Updates, error) {
	st, err := s.store.GetSettings(ctx, viewer)
	if err != nil {
		return nil, err
	}
	now := s.now()
	if st.StealthCooldown > now {
		return nil, mtproto.NewErrFloodWaitX(st.StealthCooldown - now)
	}
	if past {
		if err = s.store.HideActiveViews(ctx, viewer, now); err != nil {
			return nil, err
		}
	}
	if future || past {
		st.StealthCooldown = now + stealthCooldown
	}
	if future {
		st.StealthActive = now + stealthFutureSecs
	}
	if err = s.store.SaveSettings(ctx, st); err != nil {
		return nil, err
	}
	up := mtproto.MakeTLUpdates(&mtproto.Updates{
		Updates: []*mtproto.Update{
			mtproto.MakeTLUpdateStoriesStealthMode(&mtproto.Update{
				StealthMode: s.stealthMode(ctx, viewer),
			}).To_Update(),
		},
		Users: []*mtproto.User{},
		Chats: []*mtproto.Chat{},
		Date:  now,
	}).To_Updates()
	var exclude []int64
	if auth != 0 {
		exclude = []int64{auth}
	}
	s.push(ctx, viewer, exclude, up)
	return up, nil
}

func (s *Service) Search(ctx context.Context, viewer int64, hashtag, offset string, limit int32) (*mtproto.Stories_FoundStories, error) {
	hashtag = strings.TrimPrefix(strings.TrimSpace(hashtag), "#")
	res := mtproto.MakeTLStoriesFoundStories(&mtproto.Stories_FoundStories{
		Stories: []*mtproto.FoundStory{},
		Chats:   []*mtproto.Chat{},
		Users:   []*mtproto.User{},
	}).To_Stories_FoundStories()
	if hashtag == "" {
		return res, nil
	}
	all, err := s.store.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(hashtag)
	var found []*Record
	for _, rec := range all {
		if rec.Deleted || rec.Item == nil || !rec.Item.Public {
			continue
		}
		if !strings.Contains(strings.ToLower(rec.Item.GetCaption().GetValue()), needle) {
			continue
		}
		if err := s.canViewPeer(ctx, viewer, rec.Owner, 0); err != nil {
			continue
		}
		if !s.privacyAllows(ctx, rec, viewer) {
			continue
		}
		found = append(found, rec)
	}
	sort.Slice(found, func(i, j int) bool { return found[i].Item.Date > found[j].Item.Date })
	start := 0
	if offset != "" {
		start, _ = strconv.Atoi(offset)
	}
	if start < 0 || start > len(found) {
		start = 0
	}
	limitN := int(clampLimit(limit, 20, 50))
	end := start + limitN
	if end > len(found) {
		end = len(found)
	}
	page := found[start:end]
	var owners []Owner
	for _, rec := range page {
		res.Stories = append(res.Stories, mtproto.MakeTLFoundStory(&mtproto.FoundStory{
			Peer:  rec.Owner.Peer(),
			Story: s.present(ctx, viewer, rec, nil),
		}).To_FoundStory())
		owners = append(owners, rec.Owner)
	}
	res.Count = int32(len(found))
	res.Chats = s.chats(ctx, viewer, owners...)
	if end < len(found) {
		res.NextOffset = &wrapperspb.StringValue{Value: strconv.Itoa(end)}
	}
	return res, nil
}
