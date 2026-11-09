// Package geo converts coordinates between WGS-84 and GCJ-02.
// Telegram clients send WGS-84. AMap and Tencent LBS expect GCJ-02 inside China.
package geo

import (
	"encoding/binary"
	"hash/fnv"
	"math"
)

const (
	gcjA  = 6378245.0
	gcjEE = 0.00669342162296594323
)

// InChina reports whether the WGS-84 point lies in the GCJ-02 offset bbox.
// Points outside are left unchanged: AMap has little data there, and applying
// the offset would move them.
func InChina(lat, lng float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lng) {
		return false
	}
	return lng >= 72.004 && lng <= 137.8347 && lat >= 0.8293 && lat <= 55.8271
}

// WGS84ToGCJ02 converts a WGS-84 point to GCJ-02. Outside China it returns
// the same point.
func WGS84ToGCJ02(lat, lng float64) (float64, float64) {
	if !InChina(lat, lng) {
		return lat, lng
	}
	dLat := transformLat(lng-105.0, lat-35.0)
	dLng := transformLng(lng-105.0, lat-35.0)
	radLat := lat / 180.0 * math.Pi
	magic := math.Sin(radLat)
	magic = 1 - gcjEE*magic*magic
	sqrtMagic := math.Sqrt(magic)
	dLat = (dLat * 180.0) / ((gcjA * (1 - gcjEE)) / (magic * sqrtMagic) * math.Pi)
	dLng = (dLng * 180.0) / (gcjA / sqrtMagic * math.Cos(radLat) * math.Pi)
	return lat + dLat, lng + dLng
}

// GCJ02ToWGS84 is the iterative inverse. The residual is under a metre
// inside China. Outside China it returns the same point.
func GCJ02ToWGS84(lat, lng float64) (float64, float64) {
	if !InChina(lat, lng) {
		return lat, lng
	}
	wLat, wLng := lat, lng
	for i := 0; i < 10; i++ {
		gLat, gLng := WGS84ToGCJ02(wLat, wLng)
		wLat += lat - gLat
		wLng += lng - gLng
	}
	return wLat, wLng
}

// AccessHash is a stable non-zero hash for a WGS-84 point. Android copies
// it into upload.getWebFile. The preview accepts any hash; this only makes
// the field non-zero on the messages we send.
func AccessHash(lat, lng float64) int64 {
	h := fnv.New64a()
	var buf [16]byte
	binary.LittleEndian.PutUint64(buf[:8], math.Float64bits(lat))
	binary.LittleEndian.PutUint64(buf[8:], math.Float64bits(lng))
	_, _ = h.Write(buf[:])
	n := int64(h.Sum64() & 0x7fffffffffffffff)
	if n == 0 {
		return 1
	}
	return n
}

func transformLat(x, y float64) float64 {
	ret := -100.0 + 2.0*x + 3.0*y + 0.2*y*y + 0.1*x*y + 0.2*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(y*math.Pi) + 40.0*math.Sin(y/3.0*math.Pi)) * 2.0 / 3.0
	ret += (160.0*math.Sin(y/12.0*math.Pi) + 320*math.Sin(y*math.Pi/30.0)) * 2.0 / 3.0
	return ret
}

func transformLng(x, y float64) float64 {
	ret := 300.0 + x + 2.0*y + 0.1*x*x + 0.1*x*y + 0.1*math.Sqrt(math.Abs(x))
	ret += (20.0*math.Sin(6.0*x*math.Pi) + 20.0*math.Sin(2.0*x*math.Pi)) * 2.0 / 3.0
	ret += (20.0*math.Sin(x*math.Pi) + 40.0*math.Sin(x/3.0*math.Pi)) * 2.0 / 3.0
	ret += (150.0*math.Sin(x/12.0*math.Pi) + 300.0*math.Sin(x/30.0*math.Pi)) * 2.0 / 3.0
	return ret
}
