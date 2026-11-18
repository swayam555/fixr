package ingest

import (
	"sort"

	"fixr.in/svc/internal/geo"
)

type Fix struct {
	Lat, Lng   float64
	AccuracyM  float64
	RecordedAt int64 // epoch ms
}

type Reason string

const (
	Accepted    Reason = ""
	BadCoords   Reason = "bad_coords"
	LowAccuracy Reason = "low_accuracy"
	FromFuture  Reason = "future"
	TooOld      Reason = "too_old"
	OutOfOrder  Reason = "out_of_order"
	Teleport    Reason = "speed"
	Jitter      Reason = "jitter"
)

type Rules struct {
	MaxAccuracyM float64 // worse than this is noise
	MaxSpeedMps  float64 // faster than this between fixes is a GPS jump
	MaxSkewMs    int64   // device clock ahead of server
	MaxAgeMs     int64   // older than this is not worth routing on
	MinMoveM     float64 // closer than this AND within accuracy of last is jitter
	MinGapMs     int64   // jitter suppression only applies to fixes this close in time
}

var DefaultRules = Rules{MaxAccuracyM: 100, MaxSpeedMps: 33.3, MaxSkewMs: 60_000, MaxAgeMs: 3_600_000, MinMoveM: 5, MinGapMs: 10_000}

type Verdict struct {
	Fix    Fix
	Reason Reason
}

// Filter sorts a batch by device time and classifies each fix against the last accepted one.
// last is nil for a pro's first fix. It returns the verdicts and the new last accepted fix.
func Filter(batch []Fix, last *Fix, nowMs int64, r Rules) ([]Verdict, *Fix) {
	sorted := append([]Fix(nil), batch...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].RecordedAt < sorted[j].RecordedAt })
	out := make([]Verdict, 0, len(sorted))
	for _, f := range sorted {
		reason := classify(f, last, nowMs, r)
		out = append(out, Verdict{Fix: f, Reason: reason})
		if reason == Accepted {
			kept := f
			last = &kept
		}
	}
	return out, last
}

func classify(f Fix, last *Fix, nowMs int64, r Rules) Reason {
	switch {
	case f.Lat < -90 || f.Lat > 90 || f.Lng < -180 || f.Lng > 180 || (f.Lat == 0 && f.Lng == 0):
		return BadCoords
	case f.AccuracyM > r.MaxAccuracyM:
		return LowAccuracy
	case f.RecordedAt > nowMs+r.MaxSkewMs:
		return FromFuture
	case f.RecordedAt < nowMs-r.MaxAgeMs:
		return TooOld
	}
	if last == nil {
		return Accepted
	}
	dt := f.RecordedAt - last.RecordedAt
	if dt <= 0 {
		return OutOfOrder
	}
	d := geo.DistanceM(geo.Point{Lat: last.Lat, Lng: last.Lng}, geo.Point{Lat: f.Lat, Lng: f.Lng})
	if d/(float64(dt)/1000) > r.MaxSpeedMps && d > f.AccuracyM+last.AccuracyM {
		return Teleport
	}
	if dt < r.MinGapMs && d < r.MinMoveM {
		return Jitter
	}
	return Accepted
}
