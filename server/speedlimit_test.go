package server

import "testing"

func TestMbpsToTcRate(t *testing.T) {
	cases := []struct {
		mbPerSec float64
		want     string
	}{
		{0, "8kbit"},
		{0.0125, "100kbit"}, // UI 0.1 Мбит/с
		{0.25, "2mbit"},     // UI 2 Мбит/с
		{1.0, "8mbit"},      // UI 8 Мбит/с
		{0.001, "8kbit"},    // floor
	}
	for _, c := range cases {
		got := mbpsToTcRate(c.mbPerSec)
		if got != c.want {
			t.Fatalf("mbpsToTcRate(%v)=%q want %q", c.mbPerSec, got, c.want)
		}
	}
}

func TestTcNeedsHTBReset(t *testing.T) {
	fq := "qdisc fq 0: root refcnt 2 limit 10000p"
	if !tcNeedsHTBReset(fq) {
		t.Fatal("fq root must reset")
	}
	htb := "qdisc htb 1: root refcnt 2 r2q 10 default 0x999"
	if tcNeedsHTBReset(htb) {
		t.Fatal("clean HTB must not reset")
	}
	mixed := fq + "\n" + htb
	if !tcNeedsHTBReset(mixed) {
		t.Fatal("fq+htb dual root must reset")
	}
}
