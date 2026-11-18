// Package match assigns open orders to candidate pros for one dispatch round.
package match

import "math"

// Infeasible marks a pair that must never be assigned (skill mismatch, too far, pro excluded).
const Infeasible = math.MaxFloat64 / 4

// Hungarian solves the rectangular assignment problem (minimise total cost) in O(n²m).
// cost[i][j] is the cost of giving order i to pro j. It returns, for each order, the pro index or -1.
func Hungarian(cost [][]float64) []int {
	n := len(cost)
	if n == 0 {
		return nil
	}
	m := len(cost[0])
	transposed := n > m
	if transposed {
		cost = transpose(cost)
		n, m = m, n
	}
	// potentials u (rows), v (cols); p[j] = row matched to col j (1-indexed, 0 = none)
	u := make([]float64, n+1)
	v := make([]float64, m+1)
	p := make([]int, m+1)
	way := make([]int, m+1)
	for i := 1; i <= n; i++ {
		p[0] = i
		j0 := 0
		minv := make([]float64, m+1)
		used := make([]bool, m+1)
		for j := range minv {
			minv[j] = math.Inf(1)
		}
		for {
			used[j0] = true
			i0, delta, j1 := p[j0], math.Inf(1), 0
			for j := 1; j <= m; j++ {
				if used[j] {
					continue
				}
				cur := cost[i0-1][j-1] - u[i0] - v[j]
				if cur < minv[j] {
					minv[j], way[j] = cur, j0
				}
				if minv[j] < delta {
					delta, j1 = minv[j], j
				}
			}
			for j := 0; j <= m; j++ {
				if used[j] {
					u[p[j]] += delta
					v[j] -= delta
				} else {
					minv[j] -= delta
				}
			}
			j0 = j1
			if p[j0] == 0 {
				break
			}
		}
		for j0 != 0 {
			j1 := way[j0]
			p[j0] = p[j1]
			j0 = j1
		}
	}
	rowToCol := make([]int, n)
	for i := range rowToCol {
		rowToCol[i] = -1
	}
	for j := 1; j <= m; j++ {
		if p[j] != 0 && cost[p[j]-1][j-1] < Infeasible {
			rowToCol[p[j]-1] = j - 1
		}
	}
	if !transposed {
		return rowToCol
	}
	// rows were pros; invert back to order -> pro
	out := make([]int, m)
	for i := range out {
		out[i] = -1
	}
	for pro, order := range rowToCol {
		if order >= 0 {
			out[order] = pro
		}
	}
	return out
}

// Greedy is the first-come baseline: each order in arrival order takes its cheapest free pro.
func Greedy(cost [][]float64) []int {
	out := make([]int, len(cost))
	taken := map[int]bool{}
	for i, row := range cost {
		out[i] = -1
		best := Infeasible
		for j, c := range row {
			if !taken[j] && c < best {
				best, out[i] = c, j
			}
		}
		if out[i] >= 0 {
			taken[out[i]] = true
		}
	}
	return out
}

func Total(cost [][]float64, assign []int) (total float64, matched int) {
	for i, j := range assign {
		if j >= 0 {
			total += cost[i][j]
			matched++
		}
	}
	return
}

func transpose(c [][]float64) [][]float64 {
	t := make([][]float64, len(c[0]))
	for j := range t {
		t[j] = make([]float64, len(c))
		for i := range c {
			t[j][i] = c[i][j]
		}
	}
	return t
}
