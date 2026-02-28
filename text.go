package goappleads

import (
	"maps"
	"slices"
	"unicode"

	"github.com/ndx-technologies/geo"
)

func runeScript(r rune) string {
	switch {
	case unicode.Is(unicode.Han, r):
		return "Han"
	case unicode.Is(unicode.Hangul, r):
		return "Hangul"
	case unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r):
		return "Japanese"
	case unicode.Is(unicode.Arabic, r):
		return "Arabic"
	case unicode.Is(unicode.Cyrillic, r):
		return "Cyrillic"
	case r >= 0x0E00 && r <= 0x0E7F:
		return "Thai"
	case r > 0x007F && unicode.Is(unicode.Latin, r):
		return "" // accented Latin — fine everywhere
	default:
		return ""
	}
}

func KeywordUnexpectedScripts(kw string, allowed map[string]bool) []string {
	found := make(map[string]bool)
	for _, r := range kw {
		s := runeScript(r)
		if s == "" || allowed[s] {
			continue
		}
		found[s] = true
	}
	return slices.Sorted(maps.Keys(found))
}

func CountryAllowedScripts(countries []geo.Country) map[string]bool {
	allowed := map[string]bool{}
	for _, c := range countries {
		switch c {
		case geo.Japan:
			allowed["Han"] = true
			allowed["Japanese"] = true
		case geo.KoreaSouth:
			allowed["Hangul"] = true
		case geo.HongKong, geo.Singapore, geo.Taiwan:
			allowed["Han"] = true
		case geo.Thailand:
			allowed["Thai"] = true
		case geo.UnitedArabEmirates:
			allowed["Arabic"] = true
		}
	}
	return allowed
}
