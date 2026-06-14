package vkhash

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"abc", "abc"},
		{
			"https://vk.com/call/join/hash1, hash2",
			"hash1,hash2",
		},
		{
			"hash1\nhash2;hash3 hash4",
			"hash1,hash2,hash3,hash4",
		},
		{
			"hash1,hash1,hash2",
			"hash1,hash2",
		},
		{
			"h1,h2,h3,h4,h5",
			"h1,h2,h3,h4",
		},
	}
	for _, tc := range tests {
		if got := Normalize(tc.in); got != tc.want {
			t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatForLink(t *testing.T) {
	if got := FormatForLink("", FormatBare, 0); got != Placeholder {
		t.Fatalf("empty -> %q", got)
	}
	if got := FormatForLink("abc", FormatBare, 1); got != "abc" {
		t.Fatalf("bare: %q", got)
	}
	if got := FormatForLink("abc", FormatJoinURL, 1); got != "https://vk.com/call/join/abc" {
		t.Fatalf("join: %q", got)
	}
	if got := FormatForLink("h1,h2", FormatBare, 1); got != "h1" {
		t.Fatalf("limit: %q", got)
	}
}
