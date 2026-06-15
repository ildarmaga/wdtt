package panel

import (
	"fmt"
	"strings"
)

// minWdttReleaseWithDB — первый релиз с SQLite panel.db (v1.1.0).
const minWdttReleaseWithDB = "v1.1.0"

func normalizeReleaseTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	if !strings.HasPrefix(tag, "v") {
		return "v" + tag
	}
	return tag
}

func parseReleaseVer(tag string) (major, minor, patch int, ok bool) {
	tag = strings.TrimPrefix(normalizeReleaseTag(tag), "v")
	parts := strings.Split(tag, ".")
	if len(parts) < 2 {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[0], "%d", &major); err != nil {
		return 0, 0, 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minor); err != nil {
		return 0, 0, 0, false
	}
	if len(parts) >= 3 {
		patchPart := parts[2]
		if i := strings.IndexAny(patchPart, "-+"); i >= 0 {
			patchPart = patchPart[:i]
		}
		fmt.Sscanf(patchPart, "%d", &patch)
	}
	return major, minor, patch, true
}

func releaseTagAtLeast(tag, min string) bool {
	ma, mi, pa, ok := parseReleaseVer(tag)
	if !ok {
		return false
	}
	mb, mn, pb, ok := parseReleaseVer(min)
	if !ok {
		return true
	}
	if ma != mb {
		return ma > mb
	}
	if mi != mn {
		return mi > mn
	}
	return pa >= pb
}

func filterReleaseTagsSinceDB(tags []string) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		if releaseTagAtLeast(t, minWdttReleaseWithDB) {
			out = append(out, t)
		}
	}
	return out
}
