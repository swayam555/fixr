package ingest

import (
	"testing"

	"fixr.in/svc/internal/geo"
)

const now = int64(1_800_000_000_000)

var koramangala = geo.Point{Lat: 12.9352, Lng: 77.6245}

func at(p geo.Point, ms int64, acc float64) Fix {
	return Fix{Lat: p.Lat, Lng: p.Lng, AccuracyM: acc, RecordedAt: ms}
}

func reasons(v []Verdict) []Reason {
	out := make([]Reason, len(v))
	for i, x := range v {
		out[i] = x.Reason
	}
	return out
}

func TestFilterAcceptsAScooterAndRejectsAGPSJump(t *testing.T) {
	p0 := koramangala
	p1 := geo.Offset(p0, 30, 0)      // 30 m in 3 s = 36 km/h, fine
	jump := geo.Offset(p1, 2_000, 0) // 2 km in 3 s, a jump
	p2 := geo.Offset(p1, 60, 0)      // 60 m in 6 s since p1, fine
	batch := []Fix{at(p0, now-9000, 8), at(p1, now-6000, 8), at(jump, now-3000, 8), at(p2, now, 8)}

	v, last := Filter(batch, nil, now, DefaultRules)

	want := []Reason{Accepted, Accepted, Teleport, Accepted}
	for i, r := range reasons(v) {
		if r != want[i] {
			t.Fatalf("fix %d: got %q want %q (all %v)", i, r, want[i], reasons(v))
		}
	}
	if last.RecordedAt != now {
		t.Fatalf("last accepted should be the newest good fix")
	}
}

func TestFilterSortsOutOfOrderBatchesAndRejectsStaleAgainstLast(t *testing.T) {
	last := at(koramangala, now-1000, 5)
	batch := []Fix{at(geo.Offset(koramangala, 50, 0), now+5000, 5), at(koramangala, now-5000, 5)}

	v, _ := Filter(batch, &last, now, DefaultRules)

	if got := reasons(v); got[0] != OutOfOrder || got[1] != Accepted {
		t.Fatalf("got %v", got)
	}
}

func TestFilterQualityRules(t *testing.T) {
	cases := map[string]struct {
		fix  Fix
		want Reason
	}{
		"null island":   {Fix{Lat: 0, Lng: 0, AccuracyM: 5, RecordedAt: now}, BadCoords},
		"poor accuracy": {at(koramangala, now, 250), LowAccuracy},
		"clock ahead":   {at(koramangala, now+120_000, 5), FromFuture},
		"two hours old": {at(koramangala, now-7_200_000, 5), TooOld},
	}
	for name, c := range cases {
		v, _ := Filter([]Fix{c.fix}, nil, now, DefaultRules)
		if v[0].Reason != c.want {
			t.Errorf("%s: got %q want %q", name, v[0].Reason, c.want)
		}
	}
}

func TestFilterSuppressesStationaryJitterButNotSlowWalking(t *testing.T) {
	last := at(koramangala, now-4000, 6)
	jitter := at(geo.Offset(koramangala, 2, 1), now-2000, 6)
	walk := at(geo.Offset(koramangala, 0, 12), now, 6)

	v, _ := Filter([]Fix{jitter, walk}, &last, now, DefaultRules)

	if got := reasons(v); got[0] != Jitter || got[1] != Accepted {
		t.Fatalf("got %v", got)
	}
}

func TestBigAccuracyCirclesDoNotCountAsTeleports(t *testing.T) {
	last := at(koramangala, now-1000, 60)
	wobble := at(geo.Offset(koramangala, 80, 0), now, 60) // 80 m in 1 s, but within the combined 120 m error

	v, _ := Filter([]Fix{wobble}, &last, now, DefaultRules)

	if v[0].Reason != Accepted {
		t.Fatalf("got %q", v[0].Reason)
	}
}
