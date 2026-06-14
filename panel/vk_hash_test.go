package main

import "testing"

func TestNormalizeVkHashes(t *testing.T) {
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
		if got := normalizeVkHashes(tc.in); got != tc.want {
			t.Fatalf("normalizeVkHashes(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
