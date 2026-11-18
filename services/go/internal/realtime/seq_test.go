package realtime

import "testing"

func TestSeqLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1700000000000-0", "1700000000000-1", true},
		{"1700000000000-9", "1700000000000-10", true},
		{"1700000000001-0", "1700000000000-99", false},
		{"5-5", "5-5", false},
	}
	for _, c := range cases {
		if got := SeqLess(c.a, c.b); got != c.want {
			t.Errorf("SeqLess(%s,%s)=%v", c.a, c.b, got)
		}
	}
}
