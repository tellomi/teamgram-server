package book

import (
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

// People Nearby.
// https://core.telegram.org/method/contacts.getLocated
// https://core.telegram.org/api/nearby
//
// A user becomes visible when contacts.getLocated is called with self_expires.
// 0 hides them. 0x7fffffff means the point does not expire. Other positive
// values are a TTL in seconds. Nearby results are users currently visible
// within nearbyRadius meters. Geogroups are not indexed here.

const (
	locatedKeyPrefix = "contacts_located_v1:"
	locatedIndexKey  = "contacts_located_idx_v1"
	nearbyRadius     = 50_000
	maxNearby        = 100
	neverExpires     = int32(0x7fffffff)
)

// Neighbor is another user who is currently visible.
type Neighbor struct {
	UserID   int64
	Distance int32
	Expires  int32
}

type locRec struct {
	UserID  int64   `json:"user_id"`
	Lat     float64 `json:"lat"`
	Long    float64 `json:"long"`
	Expires int32   `json:"expires"`
}

// ParseGeo reads an inputGeoPoint. inputGeoPointEmpty and out-of-range
// coordinates are GEO_POINT_INVALID.
func ParseGeo(in *mtproto.InputGeoPoint) (lat, long float64, err error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputGeoPointEmpty || in.GetPredicateName() == "" {
		return 0, 0, mtproto.ErrGeoPointInvalid
	}
	lat, long = in.GetLat(), in.GetLong()
	if math.IsNaN(lat) || math.IsNaN(long) || lat < -90 || lat > 90 || long < -180 || long > 180 {
		return 0, 0, mtproto.ErrGeoPointInvalid
	}
	return lat, long, nil
}

// Publish stores the user's public point. expiresUnix is the unix time the
// point stops being visible, or 0x7fffffff when it does not expire.
func (b *Book) Publish(ctx context.Context, user int64, lat, long float64, expiresUnix int32) error {
	rec := locRec{UserID: user, Lat: lat, Long: long, Expires: expiresUnix}
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	if err := b.kv.SetCtx(ctx, locatedKey(user), string(raw)); err != nil {
		return err
	}
	return b.addIndex(ctx, user)
}

// Hide removes the user's public point.
func (b *Book) Hide(ctx context.Context, user int64) error {
	if _, err := b.kv.DelCtx(ctx, locatedKey(user)); err != nil {
		return err
	}
	return b.dropIndex(ctx, user)
}

// SelfExpires returns the unix expiry of the user's public point.
func (b *Book) SelfExpires(ctx context.Context, user int64, now int32) (int32, bool) {
	rec, ok := b.loadLoc(ctx, user, now)
	if !ok {
		return 0, false
	}
	return rec.Expires, true
}

// Nearby returns other visible users within nearbyRadius, closest first.
func (b *Book) Nearby(ctx context.Context, self int64, lat, long float64, now int32) []Neighbor {
	var out []Neighbor
	for _, id := range b.index(ctx) {
		if id == self {
			continue
		}
		rec, ok := b.loadLoc(ctx, id, now)
		if !ok {
			continue
		}
		meters := haversine(lat, long, rec.Lat, rec.Long)
		if meters > nearbyRadius {
			continue
		}
		out = append(out, Neighbor{UserID: id, Distance: int32(meters), Expires: rec.Expires})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Distance != out[j].Distance {
			return out[i].Distance < out[j].Distance
		}
		return out[i].UserID < out[j].UserID
	})
	if len(out) > maxNearby {
		out = out[:maxNearby]
	}
	return out
}

// ExpiresUnix turns a self_expires TTL into the unix time stored and returned
// to the client. 0x7fffffff stays as the "does not expire" sentinel.
func ExpiresUnix(now, ttl int32) int32 {
	if ttl >= neverExpires || ttl < 0 {
		return neverExpires
	}
	sum := int64(now) + int64(ttl)
	if sum > int64(neverExpires) {
		return neverExpires
	}
	return int32(sum)
}

func (b *Book) loadLoc(ctx context.Context, user int64, now int32) (locRec, bool) {
	raw, err := b.kv.GetCtx(ctx, locatedKey(user))
	if err != nil || raw == "" {
		return locRec{}, false
	}
	var rec locRec
	if json.Unmarshal([]byte(raw), &rec) != nil {
		return locRec{}, false
	}
	if rec.Expires != neverExpires && rec.Expires <= now {
		_, _ = b.kv.DelCtx(ctx, locatedKey(user))
		_ = b.dropIndex(ctx, user)
		return locRec{}, false
	}
	return rec, true
}

func (b *Book) index(ctx context.Context) []int64 {
	raw, err := b.kv.GetCtx(ctx, locatedIndexKey)
	if err != nil || raw == "" {
		return nil
	}
	var ids []int64
	if json.Unmarshal([]byte(raw), &ids) != nil {
		return nil
	}
	return ids
}

func (b *Book) addIndex(ctx context.Context, user int64) error {
	ids := b.index(ctx)
	for _, id := range ids {
		if id == user {
			return nil
		}
	}
	ids = append(ids, user)
	return b.writeIndex(ctx, ids)
}

func (b *Book) dropIndex(ctx context.Context, user int64) error {
	ids := b.index(ctx)
	kept := ids[:0]
	for _, id := range ids {
		if id != user {
			kept = append(kept, id)
		}
	}
	return b.writeIndex(ctx, kept)
}

func (b *Book) writeIndex(ctx context.Context, ids []int64) error {
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, locatedIndexKey, string(raw))
}

func locatedKey(user int64) string {
	return locatedKeyPrefix + strconv.FormatInt(user, 10)
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const earth = 6371000.0
	r := math.Pi / 180
	dLat := (lat2 - lat1) * r
	dLon := (lon2 - lon1) * r
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*r)*math.Cos(lat2*r)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earth * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
