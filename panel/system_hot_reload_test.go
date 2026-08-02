package panel

import (
	"errors"
	"testing"
)

func TestApplyWdttConfigChangeHotOnlyReturnsError(t *testing.T) {
	old := wdttHotReloadFn
	wdttHotReloadFn = func() error { return errors.New("boom") }
	defer func() { wdttHotReloadFn = old }()

	err := applyWdttConfigChangeMaybeRestart(false)
	if err == nil {
		t.Fatal("expected error on HotOnly reload failure")
	}
}
