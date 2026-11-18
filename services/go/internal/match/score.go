package match

import (
	"math"

	"fixr.in/svc/internal/geo"
)

type Order struct {
	ID         string
	Loc        geo.Point
	Skill      string
	SkillLevel int // 1 basic .. 3 expert, from the AI scope
	CreatedMs  int64
	Excluded   map[string]bool // pros who declined or timed out on this order
}

type Pro struct {
	ID          string
	Loc         geo.Point
	Skills      map[string]int // skill -> level
	Rating      float64        // 1..5
	JobsToday   int
	IdleSinceMs int64
}

type Weights struct {
	RatingSecPerStar float64 // each star below 5 costs this many seconds
	JobSec           float64 // each job already done today costs this (spreads work)
	IdleCreditSec    float64 // per idle minute, capped, rewards pros who have waited longest
	IdleCapMin       float64
	MaxEtaSec        float64
	WaitBoostSec     float64 // per minute an order has waited, makes old orders win ties
}

var DefaultWeights = Weights{RatingSecPerStar: 90, JobSec: 25, IdleCreditSec: 6, IdleCapMin: 45, MaxEtaSec: 25 * 60, WaitBoostSec: 20}

// HeuristicEtaSec is the fallback when the ETA model is unavailable: Bengaluru two-wheeler at ~18 km/h,
// a 1.35 road-network factor over straight-line distance, plus a minute to park and reach the door.
func HeuristicEtaSec(from, to geo.Point) float64 {
	return geo.DistanceM(from, to)*1.35/5.0 + 60
}

// Cost is seconds-equivalent: lower is better. Infeasible if the pro lacks the skill or is too far.
func Cost(o Order, p Pro, etaSec float64, nowMs int64, w Weights) float64 {
	level, ok := p.Skills[o.Skill]
	if !ok || level < o.SkillLevel || o.Excluded[p.ID] || etaSec > w.MaxEtaSec {
		return Infeasible
	}
	idleMin := math.Min(float64(nowMs-p.IdleSinceMs)/60_000, w.IdleCapMin)
	waitMin := float64(nowMs-o.CreatedMs) / 60_000
	c := etaSec +
		(5-p.Rating)*w.RatingSecPerStar +
		float64(p.JobsToday)*w.JobSec -
		math.Max(0, idleMin)*w.IdleCreditSec -
		waitMin*w.WaitBoostSec
	return math.Max(c, 1)
}
