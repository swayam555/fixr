package match

import (
	"testing"

	"fixr.in/svc/internal/geo"
)

func TestCostRules(t *testing.T) {
	now := int64(10_000_000)
	o := Order{ID: "o", Skill: "plumbing", SkillLevel: 2, CreatedMs: now, Excluded: map[string]bool{"declined": true}}
	pro := func(id string, level int) Pro {
		return Pro{ID: id, Skills: map[string]int{"plumbing": level}, Rating: 5, IdleSinceMs: now}
	}

	if Cost(o, pro("junior", 1), 300, now, DefaultWeights) != Infeasible {
		t.Error("an under-skilled pro must be infeasible")
	}
	if Cost(o, pro("declined", 3), 300, now, DefaultWeights) != Infeasible {
		t.Error("a pro who declined must not be re-offered the same order")
	}
	if Cost(o, pro("far", 3), 30*60, now, DefaultWeights) != Infeasible {
		t.Error("beyond MaxEta must be infeasible")
	}

	fresh := pro("fresh", 2)
	waited := pro("waited", 2)
	waited.IdleSinceMs = now - 30*60_000
	if !(Cost(o, waited, 300, now, DefaultWeights) < Cost(o, fresh, 300, now, DefaultWeights)) {
		t.Error("at equal ETA the pro idle longest should win (utilisation)")
	}
	low := pro("low", 2)
	low.Rating = 3.5
	if !(Cost(o, fresh, 300, now, DefaultWeights) < Cost(o, low, 300, now, DefaultWeights)) {
		t.Error("at equal ETA the better-rated pro should win")
	}
}

func TestHeuristicEta(t *testing.T) {
	a := geo.Point{Lat: 12.9352, Lng: 77.6245}
	b := geo.Offset(a, 2000, 0)
	got := HeuristicEtaSec(a, b)
	if got < 580 || got > 620 { // 2 km * 1.35 / 5 m/s + 60 s = 600 s
		t.Fatalf("got %v", got)
	}
}
