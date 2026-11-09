package location

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"time"

	"github.com/teamgram/teamgram-server/pkg/geo"
)

const mapTTL = 30 * 24 * time.Hour

// StaticMap is the PNG for upload.getWebFile inputWebFileGeoPointLocation.
// ok is false when TELLOMI_AMAP_KEY is unset or the map cannot be fetched.
// The caller turns that into WEBFILE_NOT_AVAILABLE.
func StaticMap(ctx context.Context, userID int64, lat, lng float64, w, h, zoom, scale int32) ([]byte, bool) {
	c := fromContext(ctx)
	if c.amapKey == "" {
		return nil, false
	}
	if math.IsNaN(lat) || math.IsNaN(lng) || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		return nil, false
	}
	lat, lng = round5(lat), round5(lng)
	w, h, zoom, scale = normMap(w, h, zoom, scale)
	key := fmt.Sprintf("venue:map:%.5f:%.5f:%d:%d:%d:%d", lat, lng, w, h, zoom, scale)
	if raw, ok := c.cache.Get(ctx, key); ok && isPNG(raw) {
		return raw, true
	}
	if !c.gate.allow(userID) {
		return nil, false
	}
	gLat, gLng := geo.WGS84ToGCJ02(lat, lng)
	body, err := c.amapStatic(ctx, gLat, gLng, w, h, zoom, scale)
	if err != nil || !isPNG(body) {
		if err == nil && quotaBody(body) {
			c.gate.open(c.now())
		}
		if c.tencentKey != "" {
			body, err = c.tencentStatic(ctx, gLat, gLng, w, h, zoom, scale)
			if err == nil && !isPNG(body) && quotaBody(body) {
				c.gate.open(c.now())
			}
		}
	}
	if err != nil || !isPNG(body) {
		return nil, false
	}
	c.cache.Set(ctx, key, body, mapTTL)
	return body, true
}

func (c *Client) amapStatic(ctx context.Context, gcjLat, gcjLng float64, w, h, zoom, scale int32) ([]byte, error) {
	q := url.Values{}
	q.Set("key", c.amapKey)
	loc := fmt.Sprintf("%.6f,%.6f", gcjLng, gcjLat)
	q.Set("location", loc)
	q.Set("zoom", strconv.Itoa(int(zoom)))
	q.Set("size", fmt.Sprintf("%d*%d", w, h))
	q.Set("scale", strconv.Itoa(int(scale)))
	q.Set("markers", "mid,,:"+loc)
	path := "/v3/staticmap"
	started := time.Now()
	body, status, err := c.get(ctx, "amap", c.amapBase+path+"?"+q.Encode(), true)
	c.logUpstream(ctx, "amap", path, strconv.Itoa(status), "", started, err)
	if status == 429 {
		c.gate.open(c.now())
	}
	return body, err
}

func (c *Client) tencentStatic(ctx context.Context, gcjLat, gcjLng float64, w, h, zoom, scale int32) ([]byte, error) {
	q := url.Values{}
	q.Set("key", c.tencentKey)
	center := fmt.Sprintf("%f,%f", gcjLat, gcjLng)
	q.Set("center", center)
	q.Set("zoom", strconv.Itoa(int(zoom)))
	q.Set("size", fmt.Sprintf("%d*%d", w, h))
	q.Set("scale", strconv.Itoa(int(scale)))
	q.Set("markers", "size:mid|color:0xFF0000|"+center)
	path := "/ws/staticmap/v2/"
	started := time.Now()
	body, status, err := c.get(ctx, "tencent", c.tencentBase+path+"?"+q.Encode(), false)
	c.logUpstream(ctx, "tencent", path, strconv.Itoa(status), "", started, err)
	if status == 429 {
		c.gate.open(c.now())
	}
	return body, err
}

func normMap(w, h, zoom, scale int32) (int32, int32, int32, int32) {
	if w <= 0 {
		w = 512
	}
	if h <= 0 {
		h = 512
	}
	if w > 1024 {
		w = 1024
	}
	if h > 1024 {
		h = 1024
	}
	if zoom < 1 {
		zoom = 1
	}
	if zoom > 17 {
		zoom = 17
	}
	if scale >= 2 {
		scale = 2
	} else {
		scale = 1
	}
	return w, h, zoom, scale
}

func round5(v float64) float64 { return math.Round(v*1e5) / 1e5 }
