package chatbox

import (
	"context"
	"encoding/json"
	"strconv"
)

func (b *Book) load(ctx context.Context, key string, dst any) error {
	if b.db == nil {
		return nil
	}
	raw, ok, err := b.db.Load(ctx, key)
	if err != nil || !ok || len(raw) == 0 {
		return err
	}
	return json.Unmarshal(raw, dst)
}

func (b *Book) save(ctx context.Context, key string, src any) error {
	if b.db == nil {
		return nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		return err
	}
	return b.db.Save(ctx, key, raw)
}

func peerKey(peerType int32, peerID int64) string {
	return strconv.FormatInt(int64(peerType), 10) + ":" + strconv.FormatInt(peerID, 10)
}

func userKey(id int64) string {
	return strconv.FormatInt(id, 10)
}
