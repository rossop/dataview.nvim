package note

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	numberRe    = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
	dateRe      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}(:\d{2})?)?$`)
	wholeLinkRe = regexp.MustCompile(`^!?\[\[([^\[\]]+)\]\]$`)
)

// ParseValue types a raw inline-field or frontmatter string the way Dataview
// does for the v1 subset: bool, number, date, link, else string.
func ParseValue(raw string) any {
	s := strings.TrimSpace(raw)
	switch {
	case s == "":
		return nil
	case strings.EqualFold(s, "true"):
		return true
	case strings.EqualFold(s, "false"):
		return false
	case numberRe.MatchString(s):
		f, _ := strconv.ParseFloat(s, 64)
		return f
	}
	if t, ok := parseDate(s); ok {
		return t
	}
	if m := wholeLinkRe.FindStringSubmatch(s); m != nil {
		return newWikiLink(m[1], strings.HasPrefix(s, "!"))
	}
	return s
}

func parseDate(s string) (time.Time, bool) {
	if !dateRe.MatchString(s) {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// normalize converts YAML-decoded values into the closed set of types
// documented on Note.
func normalize(v any) any {
	switch x := v.(type) {
	case nil, bool, float64, time.Time:
		return x
	case string:
		// YAML already typed numbers and bools; only dates and links remain
		// hidden inside strings.
		if t, ok := parseDate(strings.TrimSpace(x)); ok {
			return t
		}
		if m := wholeLinkRe.FindStringSubmatch(strings.TrimSpace(x)); m != nil {
			return newWikiLink(m[1], strings.HasPrefix(strings.TrimSpace(x), "!"))
		}
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = normalize(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[k] = normalize(e)
		}
		return out
	case map[any]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			out[toString(k)] = normalize(e)
		}
		return out
	default:
		return toString(x)
	}
}

func toString(v any) string {
	return fmt.Sprint(v)
}
