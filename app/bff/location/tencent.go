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

const tencentPageSize = 20

type tencentSearch struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Address  string `json:"address"`
		Category string `json:"category"`
		Location struct {
			Lat float64 `json:"lat"`
			Lng float64 `json:"lng"`
		} `json:"location"`
		AdInfo struct {
			Province string `json:"province"`
			City     string `json:"city"`
			District string `json:"district"`
		} `json:"ad_info"`
	} `json:"data"`
}

type tencentRegeo struct {
	Status int `json:"status"`
	Result struct {
		Address string `json:"address"`
		AdInfo  struct {
			Province string `json:"province"`
			City     string `json:"city"`
			District string `json:"district"`
		} `json:"ad_info"`
	} `json:"result"`
}

func tencentQuota(status int) bool {
	return status == 120 || status == 121
}

func (c *Client) tencentPlaces(ctx context.Context, gcjLat, gcjLng, wgsLat, wgsLng float64, query string, page int) (here *place, pois []place, quota bool, err error) {
	if c.tencentKey == "" {
		return nil, nil, false, fmt.Errorf("tencent key unset")
	}
	keywords := strings.TrimSpace(query)
	radius := 1000
	if keywords != "" {
		radius = 5000
	}
	pois, quota, err = c.tencentSearch(ctx, gcjLat, gcjLng, radius, keywords, page)
	if quota || (err != nil && keywords == "") {
		return nil, nil, quota, err
	}
	if keywords != "" && err == nil && len(pois) < 5 {
		wider, q, e := c.tencentSearch(ctx, gcjLat, gcjLng, 20000, keywords, page)
		if q {
			return nil, nil, true, e
		}
		if e == nil && len(wider) > len(pois) {
			pois = wider
		}
	}
	if page == 1 && keywords == "" {
		if h, e := c.tencentRegeo(ctx, gcjLat, gcjLng, wgsLat, wgsLng); e == nil {
			here = h
		}
	}
	return here, pois, false, err
}

func (c *Client) tencentSearch(ctx context.Context, gcjLat, gcjLng float64, radius int, keywords string, page int) ([]place, bool, error) {
	q := url.Values{}
	q.Set("key", c.tencentKey)
	q.Set("boundary", fmt.Sprintf("nearby(%f,%f,%d)", gcjLat, gcjLng, radius))
	q.Set("page_size", strconv.Itoa(tencentPageSize))
	q.Set("page_index", strconv.Itoa(page))
	q.Set("orderby", "_distance")
	if keywords != "" {
		q.Set("keyword", keywords)
	}
	path := "/ws/place/v1/search"
	started := time.Now()
	body, status, err := c.get(ctx, "tencent", c.tencentBase+path+"?"+q.Encode(), false)
	if err != nil {
		c.logUpstream(ctx, "tencent", path, strconv.Itoa(status), "", started, err)
		return nil, status == 429, err
	}
	var parsed tencentSearch
	if err = json.Unmarshal(body, &parsed); err != nil {
		c.logUpstream(ctx, "tencent", path, strconv.Itoa(status), "", started, err)
		return nil, false, err
	}
	c.logUpstream(ctx, "tencent", path, strconv.Itoa(parsed.Status), parsed.Message, started, nil)
	if tencentQuota(parsed.Status) {
		return nil, true, fmt.Errorf("tencent status %d", parsed.Status)
	}
	if parsed.Status != 0 {
		return nil, false, fmt.Errorf("tencent status %d", parsed.Status)
	}
	out := make([]place, 0, len(parsed.Data))
	for _, p := range parsed.Data {
		if p.ID == "" || strings.TrimSpace(p.Title) == "" {
			continue
		}
		wLat, wLng := geo.GCJ02ToWGS84(p.Location.Lat, p.Location.Lng)
		out = append(out, place{
			ID:        p.ID,
			Name:      strings.TrimSpace(p.Title),
			Address:   joinAddr(p.AdInfo.Province, p.AdInfo.City, p.AdInfo.District, p.Address),
			VenueType: TencentVenueType(p.Category),
			Provider:  "tencent",
			Lat:       wLat,
			Lng:       wLng,
		})
	}
	return out, false, nil
}

func (c *Client) tencentRegeo(ctx context.Context, gcjLat, gcjLng, wgsLat, wgsLng float64) (*place, error) {
	q := url.Values{}
	q.Set("key", c.tencentKey)
	q.Set("location", fmt.Sprintf("%f,%f", gcjLat, gcjLng))
	path := "/ws/geocoder/v1/"
	started := time.Now()
	body, status, err := c.get(ctx, "tencent", c.tencentBase+path+"?"+q.Encode(), false)
	if err != nil {
		c.logUpstream(ctx, "tencent", path, strconv.Itoa(status), "", started, err)
		return nil, err
	}
	var parsed tencentRegeo
	if err = json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	c.logUpstream(ctx, "tencent", path, strconv.Itoa(parsed.Status), "", started, nil)
	if parsed.Status != 0 || parsed.Result.Address == "" {
		return nil, fmt.Errorf("tencent regeo status %d", parsed.Status)
	}
	return &place{
		ID:        "here",
		Name:      "当前位置",
		Address:   parsed.Result.Address,
		VenueType: "building/default",
		Provider:  "tencent",
		Lat:       wgsLat,
		Lng:       wgsLng,
	}, nil
}
