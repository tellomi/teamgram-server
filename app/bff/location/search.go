package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/geo"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const searchTTL = 10 * time.Minute

var errLimited = errors.New("venue limited")

type cachedPage struct {
	Places []place `json:"places"`
	Next   string  `json:"next"`
}

// Search is messages.getInlineBotResults for @venue. iOS uses Apple Maps
// when that key is configured. Android uses AMap. Anything else tries Apple
// and then AMap. A missing point, a provider that is not configured, a
// limit, or an upstream failure returns an empty list. It does not return
// an RPC error, so the location picker can still send the current location.
func Search(ctx context.Context, userID int64, point *mtproto.InputGeoPoint, query, offset string) ([]*mtproto.BotInlineResult, string) {
	c := fromContext(ctx)
	lat, lng, ok := pointOf(point)
	if !ok {
		return nil, ""
	}
	plat := platformOf(ctx)
	names := route(plat, c.apple != nil, c.amapKey != "")
	for i, name := range names {
		places, next, err := c.searchNamed(ctx, name, userID, lat, lng, query, offset)
		if errors.Is(err, errLimited) {
			return nil, ""
		}
		if err != nil {
			if plat != platformUnknown {
				return nil, ""
			}
			continue
		}
		if len(places) > 0 || next != "" || plat != platformUnknown || i == len(names)-1 {
			return toInline(places), next
		}
	}
	return nil, ""
}

func (c *Client) searchNamed(ctx context.Context, provider string, userID int64, lat, lng float64, query, offset string) ([]place, string, error) {
	if provider == providerAMap {
		if pageFrom(offset) == 0 {
			return nil, "", nil
		}
	}
	key := cacheKey(provider, lat, lng, query, offset)
	if places, next, ok := c.cached(ctx, key); ok {
		return places, next, nil
	}
	return c.single(key, func() ([]place, string, error) {
		if places, next, ok := c.cached(ctx, key); ok {
			return places, next, nil
		}
		if !c.gate.allow(userID) {
			return nil, "", errLimited
		}
		if provider == providerApple {
			return c.fetchApple(ctx, key, lat, lng, query, offset)
		}
		return c.fetch(ctx, key, lat, lng, query, pageFrom(offset))
	})
}

func cacheKey(provider string, lat, lng float64, query, offset string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(query))))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(offset))
	return fmt.Sprintf("venue:%s:%.3f:%.3f:%x", provider, round3(lat), round3(lng), h.Sum64())
}

func (c *Client) cached(ctx context.Context, key string) ([]place, string, bool) {
	raw, ok := c.cache.Get(ctx, key)
	if !ok {
		return nil, "", false
	}
	var page cachedPage
	if json.Unmarshal(raw, &page) != nil {
		return nil, "", false
	}
	return page.Places, page.Next, true
}

func (c *Client) fetch(ctx context.Context, key string, lat, lng float64, query string, page int) ([]place, string, error) {
	gLat, gLng := geo.WGS84ToGCJ02(lat, lng)
	here, pois, quota, err := c.amapPlaces(ctx, gLat, gLng, query, page)
	if quota {
		c.gate.open(c.now())
	}
	if c.tencentKey != "" && (err != nil || quota || (here == nil && len(pois) == 0)) {
		tHere, tPois, tQuota, tErr := c.tencentPlaces(ctx, gLat, gLng, lat, lng, query, page)
		if tQuota {
			c.gate.open(c.now())
		}
		if tErr == nil && (tHere != nil || len(tPois) > 0) {
			here, pois, err = tHere, tPois, nil
		}
	}
	if err != nil {
		return nil, "", err
	}
	if here != nil {
		here.Lat, here.Lng = lat, lng
	}
	places := make([]place, 0, len(pois)+1)
	if here != nil {
		places = append(places, *here)
	}
	places = append(places, pois...)
	pageSize := amapPageSize
	if len(pois) > 0 && pois[0].Provider == "tencent" {
		pageSize = tencentPageSize
	}
	next := ""
	if page < 3 && len(pois) >= pageSize {
		next = strconv.Itoa(page + 1)
	}
	c.store(ctx, key, places, next)
	return places, next, nil
}

func (c *Client) store(ctx context.Context, key string, places []place, next string) {
	blob, err := json.Marshal(cachedPage{Places: places, Next: next})
	if err != nil {
		return
	}
	ttl := searchTTL
	if len(places) == 0 {
		ttl = time.Minute
	}
	c.cache.Set(ctx, key, blob, ttl)
}

func toInline(places []place) []*mtproto.BotInlineResult {
	seen := map[string]bool{}
	out := make([]*mtproto.BotInlineResult, 0, len(places))
	for _, p := range places {
		name := strings.TrimSpace(p.Name)
		if name == "" {
			continue
		}
		id := uniqueID(p.ID, seen)
		gp := mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{Lat: p.Lat, Long: p.Lng}).To_GeoPoint()
		Stamp(gp)
		provider := p.Provider
		if provider == "" {
			provider = "amap"
		}
		vtype := p.VenueType
		if vtype == "" {
			vtype = "building/default"
		}
		var desc *wrapperspb.StringValue
		if p.Address != "" {
			desc = wrapperspb.String(p.Address)
		}
		out = append(out, mtproto.MakeTLBotInlineResult(&mtproto.BotInlineResult{
			Id:          id,
			Type:        "venue",
			Title:       wrapperspb.String(name),
			Description: desc,
			SendMessage: mtproto.MakeTLBotInlineMessageMediaVenue(&mtproto.BotInlineMessage{
				Geo:       gp,
				Title:     name,
				Address:   p.Address,
				Provider:  provider,
				VenueId:   p.ID,
				VenueType: vtype,
			}).To_BotInlineMessage(),
		}).To_BotInlineResult())
	}
	return out
}

func uniqueID(id string, seen map[string]bool) string {
	id = strings.Map(func(r rune) rune {
		if r == 0 {
			return -1
		}
		return r
	}, strings.TrimSpace(id))
	if id == "" {
		id = "poi"
	}
	if len(id) > 60 {
		id = id[:60]
	}
	base := id
	for n := 2; seen[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
		if len(id) > 64 {
			id = id[:64]
		}
	}
	seen[id] = true
	return id
}

// Stamp sets a non-zero access_hash on a real geo point.
func Stamp(g *mtproto.GeoPoint) {
	if g == nil || g.GetPredicateName() == mtproto.Predicate_geoPointEmpty {
		return
	}
	if g.AccessHash != 0 {
		return
	}
	g.AccessHash = geo.AccessHash(g.GetLat(), g.GetLong())
}

func pointOf(g *mtproto.InputGeoPoint) (float64, float64, bool) {
	if g == nil {
		return 0, 0, false
	}
	switch g.GetPredicateName() {
	case mtproto.Predicate_inputGeoPoint:
	case "":
		if g.GetLat() == 0 && g.GetLong() == 0 {
			return 0, 0, false
		}
	default:
		return 0, 0, false
	}
	lat, lng := g.GetLat(), g.GetLong()
	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return 0, 0, false
	}
	return lat, lng, true
}

func pageFrom(offset string) int {
	offset = strings.TrimSpace(offset)
	if offset == "" {
		return 1
	}
	n, err := strconv.Atoi(offset)
	if err != nil || n < 1 || n > 3 {
		return 0
	}
	return n
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
