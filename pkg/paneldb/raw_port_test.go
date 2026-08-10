package paneldb

import "testing"

func TestEffectiveRawDirectPort(t *testing.T) {
	if got := EffectiveRawDirectPort(56000, 0); got != 56003 {
		t.Fatalf("auto: got %d", got)
	}
	if got := EffectiveRawDirectPort(56000, 56111); got != 56111 {
		t.Fatalf("explicit: got %d", got)
	}
}
