package geo

import "math"

const earthRadiusM = 6_371_008.8

type Point struct{ Lat, Lng float64 }

func rad(d float64) float64 { return d * math.Pi / 180 }

// DistanceM is the haversine great-circle distance in metres.
func DistanceM(a, b Point) float64 {
	dLat, dLng := rad(b.Lat-a.Lat), rad(b.Lng-a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusM * math.Asin(math.Min(1, math.Sqrt(h)))
}

// BearingDeg is the initial bearing from a to b, 0..360 clockwise from north.
func BearingDeg(a, b Point) float64 {
	y := math.Sin(rad(b.Lng-a.Lng)) * math.Cos(rad(b.Lat))
	x := math.Cos(rad(a.Lat))*math.Sin(rad(b.Lat)) - math.Sin(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Cos(rad(b.Lng-a.Lng))
	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

// Offset moves p by metres north and east (small-distance approximation, fine below ~10 km).
func Offset(p Point, northM, eastM float64) Point {
	return Point{
		Lat: p.Lat + northM/earthRadiusM*180/math.Pi,
		Lng: p.Lng + eastM/(earthRadiusM*math.Cos(rad(p.Lat)))*180/math.Pi,
	}
}
