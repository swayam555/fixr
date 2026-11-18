package match

import (
	"math/rand/v2"
	"testing"
)

const X = Infeasible

func TestBatchBeatsGreedyOnTheClassicCrossover(t *testing.T) {
	// Order A arrives first and greedily grabs pro 1 (4 min) although pro 2 is nearly as close (5 min);
	// order B is then left with pro 3 (20 min). Batching gives A pro 2 and B pro 1.
	cost := [][]float64{
		{240, 300, 900},
		{260, 1100, 1200},
	}
	g, gm := Total(cost, Greedy(cost))
	h, hm := Total(cost, Hungarian(cost))
	if gm != 2 || hm != 2 {
		t.Fatalf("both should match 2 orders: greedy %d hungarian %d", gm, hm)
	}
	if !(h < g) || h != 560 {
		t.Fatalf("hungarian %v should beat greedy %v and equal 560", h, g)
	}
}

func TestInfeasiblePairsAreNeverAssigned(t *testing.T) {
	cost := [][]float64{
		{X, X},
		{100, X},
	}
	got := Hungarian(cost)
	if got[0] != -1 || got[1] != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestMoreOrdersThanPros(t *testing.T) {
	cost := [][]float64{{50}, {10}, {30}}
	got := Hungarian(cost)
	if got[1] != 0 || got[0] != -1 || got[2] != -1 {
		t.Fatalf("the single pro should go to the cheapest order, got %v", got)
	}
}

func TestHungarianIsOptimalAgainstBruteForce(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 7))
	for trial := 0; trial < 300; trial++ {
		n, m := 1+r.IntN(5), 1+r.IntN(5)
		cost := make([][]float64, n)
		for i := range cost {
			cost[i] = make([]float64, m)
			for j := range cost[i] {
				cost[i][j] = float64(r.IntN(1000))
				if r.IntN(6) == 0 {
					cost[i][j] = X
				}
			}
		}
		assign := Hungarian(cost)
		seen := map[int]bool{}
		for _, j := range assign {
			if j >= 0 && seen[j] {
				t.Fatalf("pro %d assigned twice: %v", j, assign)
			}
			seen[j] = true
		}
		gotTotal, gotMatched := Total(cost, assign)
		bestTotal, bestMatched := brute(cost)
		if gotMatched != bestMatched || gotTotal != bestTotal {
			t.Fatalf("trial %d: got (%d, %v) want (%d, %v) for %v", trial, gotMatched, gotTotal, bestMatched, bestTotal, cost)
		}
	}
}

// brute maximises matches first, then minimises cost, over every assignment.
func brute(cost [][]float64) (float64, int) {
	bestM, bestT := -1, 0.0
	used := make([]bool, len(cost[0]))
	var rec func(i, matched int, total float64)
	rec = func(i, matched int, total float64) {
		if i == len(cost) {
			if matched > bestM || (matched == bestM && total < bestT) {
				bestM, bestT = matched, total
			}
			return
		}
		rec(i+1, matched, total)
		for j := range cost[i] {
			if !used[j] && cost[i][j] < X {
				used[j] = true
				rec(i+1, matched+1, total+cost[i][j])
				used[j] = false
			}
		}
	}
	rec(0, 0, 0)
	return bestT, bestM
}
