package panel

import "testing"

func TestReleaseTagAtLeast(t *testing.T) {
	cases := []struct {
		tag, min string
		want     bool
	}{
		{"v1.4.0", "v1.4.0", true},
		{"v1.4.1", "v1.4.0", true},
		{"v1.3.4", "v1.4.0", false},
		{"1.4.0", "v1.4.0", true},
	}
	for _, c := range cases {
		if got := releaseTagAtLeast(c.tag, c.min); got != c.want {
			t.Fatalf("%s >= %s: got %v want %v", c.tag, c.min, got, c.want)
		}
	}
}
