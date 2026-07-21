package vkhash

import "strings"

const (
	Max         = 4
	Placeholder = "VK_HASH"
	// JoinURLBase — ссылка, которую кладём в colon/qwdtt (VK мигрирует vk.com→vk.ru).
	JoinURLBase = "https://vk.ru/call/join/"
)

// Format — как хеши подставляются в colon / qwdtt ссылки.
type Format int

const (
	FormatBare Format = iota
	FormatJoinURL
)

// StripOne извлекает bare-токен из хеша или ссылки …/call/join/….
// Accept-both: vk.com и vk.ru (и m.*) — VK мигрирует домены.
func StripOne(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	lower := strings.ToLower(s)
	if idx := strings.Index(lower, "/call/join/"); idx >= 0 {
		s = s[idx+len("/call/join/"):]
	} else if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return ""
	} else {
		prefixes := []string{
			"https://vk.ru/call/join/", "http://vk.ru/call/join/",
			"https://vk.com/call/join/", "http://vk.com/call/join/",
			"https://m.vk.ru/call/join/", "http://m.vk.ru/call/join/",
			"https://m.vk.com/call/join/", "http://m.vk.com/call/join/",
			"m.vk.ru/call/join/", "vk.ru/call/join/",
			"m.vk.com/call/join/", "vk.com/call/join/",
			"https://vk.me/join/", "http://vk.me/join/", "vk.me/join/",
		}
		for _, p := range prefixes {
			if strings.HasPrefix(lower, p) {
				s = s[len(p):]
				break
			}
		}
	}
	if i := strings.IndexAny(s, "?#/"); i >= 0 {
		s = s[:i]
	}
	return strings.Trim(s, "/ ")
}

// Parse возвращает до maxBare bare-хешей (0 = Max).
func Parse(raw string, maxBare int) []string {
	if maxBare <= 0 {
		maxBare = Max
	}
	var out []string
	seen := make(map[string]struct{})
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t' || r == ' '
	}) {
		h := StripOne(part)
		if h == "" {
			continue
		}
		if _, ok := seen[h]; ok {
			continue
		}
		seen[h] = struct{}{}
		out = append(out, h)
		if len(out) >= maxBare {
			break
		}
	}
	return out
}

// Normalize — строка для хранения в БД: bare-токены через запятую (до Max).
func Normalize(raw string) string {
	return strings.Join(Parse(raw, Max), ",")
}

// FormatForLink — для colon/qwdtt: bare или join-url; limit=1 для iOS.
func FormatForLink(raw string, format Format, limit int) string {
	list := Parse(raw, limit)
	if len(list) == 0 {
		return Placeholder
	}
	if format == FormatJoinURL {
		urls := make([]string, len(list))
		for i, h := range list {
			urls[i] = JoinURLBase + h
		}
		return strings.Join(urls, ",")
	}
	return strings.Join(list, ",")
}
