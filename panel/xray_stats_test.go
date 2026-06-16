package panel

import "testing"

func TestIsInternalXrayAccessLine(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"2026/06/16 15:10:02 from 127.0.0.1:58142 accepted tcp:127.0.0.1:62789 [api -> api]", true},
		{"2026/06/16 15:10:02 from 10.66.66.2:12345 accepted tcp:vk.com:443 [redirect-in -> direct]", false},
		{"2026/06/16 10:11:35 from DNS accepted udp:1.1.1.1:53 [xray.system.uuid -> NL]", false},
	}
	for _, tc := range cases {
		if got := isInternalXrayAccessLine(tc.line, defaultXrayAPIPort); got != tc.want {
			t.Fatalf("line=%q got=%v want=%v", tc.line, got, tc.want)
		}
	}
}
