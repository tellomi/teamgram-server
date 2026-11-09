package geo

import (
	"math"
	"testing"
)

func TestWGS84ToGCJ02Tiananmen(t *testing.T) {
	lat, lng := WGS84ToGCJ02(39.9087, 116.3975)
	if math.Abs(lat-39.91010349934476) > 1e-9 || math.Abs(lng-116.40374357265176) > 1e-9 {
		t.Fatalf("gcj %v,%v", lat, lng)
	}
	backLat, backLng := GCJ02ToWGS84(lat, lng)
	if haversine(39.9087, 116.3975, backLat, backLng) > 1 {
		t.Fatalf("inverse moved %.3f m to %v,%v", haversine(39.9087, 116.3975, backLat, backLng), backLat, backLng)
	}
}

func TestWGS84ToGCJ02Shanghai(t *testing.T) {
	lat, lng := WGS84ToGCJ02(31.1774276, 121.5272106)
	if math.Abs(lat-31.17530398364597) > 1e-9 || math.Abs(lng-121.531541859215) > 1e-9 {
		t.Fatalf("gcj %v,%v", lat, lng)
	}
}

func TestOutsideChinaUnchanged(t *testing.T) {
	lat, lng := WGS84ToGCJ02(40.7128, -74.0060)
	if lat != 40.7128 || lng != -74.0060 {
		t.Fatalf("nyc %v,%v", lat, lng)
	}
	lat, lng = GCJ02ToWGS84(51.5074, -0.1278)
	if lat != 51.5074 || lng != -0.1278 {
		t.Fatalf("london %v,%v", lat, lng)
	}
	if InChina(22.3, 114.2) != true || InChina(35.6, 139.7) != false {
		t.Fatal("bbox")
	}
}

func TestAccessHashNonZero(t *testing.T) {
	if AccessHash(39.9087, 116.3975) == 0 {
		t.Fatal("zero hash")
	}
	if AccessHash(39.9087, 116.3975) != AccessHash(39.9087, 116.3975) {
		t.Fatal("unstable")
	}
	if AccessHash(0, 0) == AccessHash(1, 1) {
		t.Fatal("collision")
	}
}

func haversine(lat1, lng1, lat2, lng2 float64) float64 {
	const earth = 6371000.0
	r := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := r(lat2 - lat1)
	dLng := r(lng2 - lng1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(r(lat1))*math.Cos(r(lat2))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earth * math.Asin(math.Sqrt(a))
}
