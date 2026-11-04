package channel

import (
	"context"
	"errors"
	"sort"
)

// errInviteExists is a duplicate link. Export retries with a new hash.
var errInviteExists = errors.New("invite link exists")

// StoredInvite is one exported chat invite for a channel or supergroup.
// Link is the hash, not the tell.cc URL.
type StoredInvite struct {
	ID            int64
	ChannelID     int64
	AdminID       int64
	Link          string
	Permanent     bool
	Revoked       bool
	RequestNeeded bool
	StartDate     int64
	ExpireDate    int64
	UsageLimit    int32
	Title         string
	Date          int64
}

func (s *StoredInvite) clone() *StoredInvite {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

// StoredImporter is one person who joined or requested to join through a link.
type StoredImporter struct {
	ID         int64
	ChannelID  int64
	Link       string
	UserID     int64
	Requested  bool
	ApprovedBy int64
	About      string
	Date       int64
}

func (s *StoredImporter) clone() *StoredImporter {
	if s == nil {
		return nil
	}
	cp := *s
	return &cp
}

func (s *memoryStore) ensureInvitesLocked() {
	if s.invites == nil {
		s.invites = map[string]*StoredInvite{}
	}
	if s.importers == nil {
		s.importers = map[string]map[int64]*StoredImporter{}
	}
}

func (s *memoryStore) InsertInvite(ctx context.Context, inv *StoredInvite) error {
	if inv == nil || inv.Link == "" || inv.ChannelID == 0 {
		return errInviteExists
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	if _, ok := s.invites[inv.Link]; ok {
		return errInviteExists
	}
	s.nextInvite++
	cp := inv.clone()
	cp.ID = s.nextInvite
	s.invites[cp.Link] = cp
	return nil
}

func (s *memoryStore) UpdateInvite(ctx context.Context, inv *StoredInvite) error {
	if inv == nil || inv.Link == "" {
		return errInviteExists
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	cur := s.invites[inv.Link]
	if cur == nil || cur.ChannelID != inv.ChannelID {
		return errInviteExists
	}
	cp := inv.clone()
	cp.ID = cur.ID
	s.invites[cp.Link] = cp
	return nil
}

func (s *memoryStore) GetInviteByLink(ctx context.Context, link string) (*StoredInvite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	return s.invites[link].clone(), nil
}

func (s *memoryStore) ListInvites(ctx context.Context, channelID int64) ([]*StoredInvite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	out := make([]*StoredInvite, 0)
	for _, inv := range s.invites {
		if inv.ChannelID == channelID {
			out = append(out, inv.clone())
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date == out[j].Date {
			return out[i].ID > out[j].ID
		}
		return out[i].Date > out[j].Date
	})
	return out, nil
}

func (s *memoryStore) DeleteInvite(ctx context.Context, channelID int64, link string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	cur := s.invites[link]
	if cur == nil || cur.ChannelID != channelID {
		return nil
	}
	delete(s.invites, link)
	delete(s.importers, link)
	return nil
}

func (s *memoryStore) DeleteRevokedInvites(ctx context.Context, channelID, adminID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	for link, inv := range s.invites {
		if inv.ChannelID == channelID && inv.AdminID == adminID && inv.Revoked {
			delete(s.invites, link)
			delete(s.importers, link)
		}
	}
	return nil
}

func (s *memoryStore) UpsertImporter(ctx context.Context, row *StoredImporter) error {
	if row == nil || row.Link == "" || row.UserID == 0 {
		return errInviteExists
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	if s.importers[row.Link] == nil {
		s.importers[row.Link] = map[int64]*StoredImporter{}
	}
	cp := row.clone()
	if cur := s.importers[row.Link][row.UserID]; cur != nil {
		cp.ID = cur.ID
	} else {
		s.nextImporter++
		cp.ID = s.nextImporter
	}
	s.importers[row.Link][row.UserID] = cp
	return nil
}

func (s *memoryStore) ListImporters(ctx context.Context, channelID int64) ([]*StoredImporter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	out := make([]*StoredImporter, 0)
	for _, byUser := range s.importers {
		for _, row := range byUser {
			if row.ChannelID == channelID {
				out = append(out, row.clone())
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Date == out[j].Date {
			return out[i].UserID > out[j].UserID
		}
		return out[i].Date > out[j].Date
	})
	return out, nil
}

func (s *memoryStore) DeleteImporter(ctx context.Context, link string, userID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInvitesLocked()
	if s.importers[link] == nil {
		return nil
	}
	delete(s.importers[link], userID)
	return nil
}
