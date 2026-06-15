package server

import (
	"testing"
	"time"
)

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "0м"},
		{5 * time.Minute, "5м"},
		{2*time.Hour + 15*time.Minute, "2ч 15м"},
		{26*time.Hour + 3*time.Minute, "1д 2ч 3м"},
	}
	for _, tc := range tests {
		if got := formatUptime(tc.d); got != tc.want {
			t.Fatalf("formatUptime(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
