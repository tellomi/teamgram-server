package location

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/geo"
)

const amapPageSize = 25

type place struct {
	ID        string
	Name      string
	Address   string
	VenueType string
	Provider  string
	Lat       float64
	Lng       float64
}

type amapPOI struct {
	ID       string     `json:"id"`
	Name     string     `json:"name"`
	Typecode string     `json:"typecode"`
	Address  flexString `json:"address"`
	Location string     `json:"location"`
	PName    string     `json:"pname"`
	City     string     `json:"cityname"`
	AdName   string     `json:"adname"`
}

type amapSearch struct {
	Status   string    `json:"status"`
	Info     string    `json:"info"`
	Infocode string    `json:"infocode"`
	Pois     []amapPOI `json:"pois"`
}

type amapRegeo struct {
	Status    string `json:"status"`
	Infocode  string `json:"infocode"`
	Regeocode struct {
		Formatted flexString `json:"formatted_address"`
		Address   struct {
			Province flexString `json:"province"`
			City     flexString `json:"city"`
			District flexString `json:"district"`
			Adcode   flexString `json:"adcode"`
		} `json:"addressComponent"`
	} `json:"regeocode"`
}

func amapQuota(code string) bool {
	switch code {
	case "10003", "10004", "10044":
		return true
	default:
		return false
	}
}

func (c *Client) amapPlaces(ctx context.Context, gcjLat, gcjLng float64, query string, page int) (here *place, pois []place, quota bool, err error) {
	adcode := ""
	if page == 1 && strings.TrimSpace(query) == "" {
		h, code, q, e := c.amapRegeo(ctx, gcjLat, gcjLng)
		if q {
			return nil, nil, true, e
		}
		if e == nil {
			here = h
			adcode = code
		}
	}
	radius := 1000
	keywords := strings.TrimSpace(query)
	if keywords != "" {
		radius = 5000
	}
	pois, quota, err = c.amapAround(ctx, gcjLat, gcjLng, radius, keywords, page)
	if quota {
		return nil, nil, true, err
	}
	if keywords != "" && err == nil && len(pois) < 5 {
		text, q, e := c.amapText(ctx, gcjLat, gcjLng, keywords, adcode, page)
		if q {
			return nil, nil, true, e
		}
		if e == nil && len(text) > len(pois) {
			pois = text
		}
	}
	return here, pois, false, err
}

func (c *Client) amapAround(ctx context.Context, gcjLat, gcjLng float64, radius int, keywords string, page int) ([]place, bool, error) {
	q := url.Values{}
	q.Set("key", c.amapKey)
	q.Set("location", fmt.Sprintf("%.6f,%.6f", gcjLng, gcjLat))
	q.Set("radius", strconv.Itoa(radius))
	q.Set("sortrule", "distance")
	q.Set("page_size", strconv.Itoa(amapPageSize))
	q.Set("page_num", strconv.Itoa(page))
	q.Set("show_fields", "business")
	if keywords != "" {
		q.Set("keywords", keywords)
	}
	return c.amapSearch(ctx, "/v5/place/around", q)
}

func (c *Client) amapText(ctx context.Context, gcjLat, gcjLng float64, keywords, adcode string, page int) ([]place, bool, error) {
	q := url.Values{}
	q.Set("key", c.amapKey)
	q.Set("keywords", keywords)
	q.Set("location", fmt.Sprintf("%.6f,%.6f", gcjLng, gcjLat))
	if adcode != "" {
		q.Set("region", adcode)
	}
	q.Set("city_limit", "false")
	q.Set("page_size", strconv.Itoa(amapPageSize))
	q.Set("page_num", strconv.Itoa(page))
	return c.amapSearch(ctx, "/v5/place/text", q)
}

func (c *Client) amapSearch(ctx context.Context, path string, q url.Values) ([]place, bool, error) {
	started := time.Now()
	raw := c.amapBase + path + "?" + q.Encode()
	body, status, err := c.get(ctx, "amap", raw, true)
	if err != nil {
		c.logUpstream(ctx, "amap", path, strconv.Itoa(status), "", started, err)
		if status == 429 {
			return nil, true, err
		}
		return nil, false, err
	}
	var parsed amapSearch
	if err = json.Unmarshal(body, &parsed); err != nil {
		c.logUpstream(ctx, "amap", path, strconv.Itoa(status), "", started, err)
		return nil, false, err
	}
	c.logUpstream(ctx, "amap", path, parsed.Status, parsed.Infocode, started, nil)
	if amapQuota(parsed.Infocode) {
		return nil, true, fmt.Errorf("amap infocode %s", parsed.Infocode)
	}
	if parsed.Status != "1" {
		return nil, false, fmt.Errorf("amap status %s", parsed.Status)
	}
	return amapPOIs(parsed.Pois), false, nil
}

func (c *Client) amapRegeo(ctx context.Context, gcjLat, gcjLng float64) (*place, string, bool, error) {
	key := fmt.Sprintf("venue:regeo:%.3f:%.3f", gcjLat, gcjLng)
	if raw, ok := c.cache.Get(ctx, key); ok {
		var cached struct {
			Place  place
			Adcode string
		}
		if json.Unmarshal(raw, &cached) == nil && cached.Place.Name != "" {
			return &cached.Place, cached.Adcode, false, nil
		}
	}
	q := url.Values{}
	q.Set("key", c.amapKey)
	q.Set("location", fmt.Sprintf("%.6f,%.6f", gcjLng, gcjLat))
	q.Set("extensions", "base")
	started := time.Now()
	body, status, err := c.get(ctx, "amap", c.amapBase+"/v3/geocode/regeo?"+q.Encode(), true)
	if err != nil {
		c.logUpstream(ctx, "amap", "/v3/geocode/regeo", strconv.Itoa(status), "", started, err)
		return nil, "", status == 429, err
	}
	var parsed amapRegeo
	if err = json.Unmarshal(body, &parsed); err != nil {
		c.logUpstream(ctx, "amap", "/v3/geocode/regeo", strconv.Itoa(status), "", started, err)
		return nil, "", false, err
	}
	c.logUpstream(ctx, "amap", "/v3/geocode/regeo", parsed.Status, parsed.Infocode, started, nil)
	if amapQuota(parsed.Infocode) {
		return nil, "", true, fmt.Errorf("amap infocode %s", parsed.Infocode)
	}
	if parsed.Status != "1" || parsed.Regeocode.Formatted.String() == "" {
		return nil, "", false, fmt.Errorf("amap regeo empty")
	}
	wLat, wLng := geo.GCJ02ToWGS84(gcjLat, gcjLng)
	p := &place{
		ID:        "here",
		Name:      "当前位置",
		Address:   parsed.Regeocode.Formatted.String(),
		VenueType: "building/default",
		Provider:  "amap",
		Lat:       wLat,
		Lng:       wLng,
	}
	blob, _ := json.Marshal(struct {
		Place  place
		Adcode string
	}{*p, parsed.Regeocode.Address.Adcode.String()})
	c.cache.Set(ctx, key, blob, 24*time.Hour)
	return p, parsed.Regeocode.Address.Adcode.String(), false, nil
}

func amapPOIs(in []amapPOI) []place {
	out := make([]place, 0, len(in))
	for _, p := range in {
		lng, lat, ok := splitLngLat(p.Location)
		if !ok || p.ID == "" || strings.TrimSpace(p.Name) == "" {
			continue
		}
		wLat, wLng := geo.GCJ02ToWGS84(lat, lng)
		out = append(out, place{
			ID:        p.ID,
			Name:      strings.TrimSpace(p.Name),
			Address:   joinAddr(p.PName, p.City, p.AdName, p.Address.String()),
			VenueType: VenueType(p.Typecode),
			Provider:  "amap",
			Lat:       wLat,
			Lng:       wLng,
		})
	}
	return out
}

func splitLngLat(s string) (lng, lat float64, ok bool) {
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lng, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lat, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return lng, lat, true
}
