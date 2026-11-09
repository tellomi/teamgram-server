package bots

import (
	"context"
	"errors"
	"strings"

	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

var errNoUserLookup = errors.New("tellomi ai: username lookup is not configured")

// usernameLister is the user-service lookup the allowlist needs.
// *user/client.defaultUserClient implements it.
type usernameLister interface {
	UserGetListByUsernameList(ctx context.Context, in *user.TLUserGetListByUsernameList) (*user.Vector_UsernameData, error)
}

// userDirectory resolves allowlist usernames through user.getListByUsernameList.
// Only user peers are returned. A channel that happens to own the name is skipped.
type userDirectory struct {
	c usernameLister
}

func (d userDirectory) Users(ctx context.Context, names []string) (map[string]int64, error) {
	out := map[string]int64{}
	if d.c == nil {
		return nil, errNoUserLookup
	}
	if len(names) == 0 {
		return out, nil
	}
	rows, err := d.c.UserGetListByUsernameList(ctx, &user.TLUserGetListByUsernameList{Names: names})
	if err != nil {
		return nil, err
	}
	if rows == nil {
		return out, nil
	}
	for _, row := range rows.GetDatas() {
		if row == nil || row.GetPeer() == nil || row.GetPeer().GetUserId() == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(row.GetUsername()), "@"))
		if name == "" {
			continue
		}
		out[name] = row.GetPeer().GetUserId()
	}
	return out, nil
}
