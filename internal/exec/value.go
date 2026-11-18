package exec

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rossop/dataview.nvim/internal/note"
)

// Runtime values are the note value set (nil, bool, float64, string,
// time.Time, note.Link, []any, map[string]any) plus Duration.

// Duration keeps months separately because months and years have no fixed
// length; days and smaller are exact.
type Duration struct {
	Months int
	D      time.Duration
}

func (d Duration) approx() time.Duration {
	return d.D + time.Duration(d.Months)*30*24*time.Hour
}

var durPart = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*([a-z]+)`)

func parseDuration(s string) (Duration, bool) {
	var d Duration
	matches := durPart.FindAllStringSubmatch(s, -1)
	if len(matches) == 0 {
		return d, false
	}
	for _, m := range matches {
		n, _ := strconv.ParseFloat(m[1], 64)
		switch u := strings.ToLower(m[2]); u {
		case "s", "sec", "secs", "second", "seconds":
			d.D += time.Duration(n * float64(time.Second))
		case "m", "min", "mins", "minute", "minutes":
			d.D += time.Duration(n * float64(time.Minute))
		case "h", "hr", "hrs", "hour", "hours":
			d.D += time.Duration(n * float64(time.Hour))
		case "d", "day", "days":
			d.D += time.Duration(n * 24 * float64(time.Hour))
		case "w", "wk", "wks", "week", "weeks":
			d.D += time.Duration(n * 7 * 24 * float64(time.Hour))
		case "mo", "month", "months":
			d.Months += int(n)
		case "y", "yr", "yrs", "year", "years":
			d.Months += int(n * 12)
		default:
			return Duration{}, false
		}
	}
	return d, true
}

func truthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case float64:
		return x != 0
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	case Duration:
		return x.approx() != 0
	}
	return true
}

// typeRank orders values of different types so sorting mixed columns is
// deterministic; null sorts first, as in Dataview.
func typeRank(v any) int {
	switch v.(type) {
	case nil:
		return 0
	case bool:
		return 1
	case float64:
		return 2
	case Duration:
		return 3
	case time.Time:
		return 4
	case string:
		return 5
	case note.Link:
		return 6
	case []any:
		return 7
	case map[string]any:
		return 8
	}
	return 9
}

// compare returns -1, 0 or 1.
func compare(a, b any) int {
	ra, rb := typeRank(a), typeRank(b)
	if ra != rb {
		return cmpInt(ra, rb)
	}
	switch x := a.(type) {
	case nil:
		return 0
	case bool:
		y := b.(bool)
		if x == y {
			return 0
		}
		if !x {
			return -1
		}
		return 1
	case float64:
		y := b.(float64)
		switch {
		case x < y:
			return -1
		case x > y:
			return 1
		}
		return 0
	case Duration:
		return cmpInt64(int64(x.approx()), int64(b.(Duration).approx()))
	case time.Time:
		return x.Compare(b.(time.Time))
	case string:
		return strings.Compare(x, b.(string))
	case note.Link:
		return strings.Compare(strings.ToLower(x.Path), strings.ToLower(b.(note.Link).Path))
	case []any:
		y := b.([]any)
		for i := 0; i < len(x) && i < len(y); i++ {
			if c := compare(x[i], y[i]); c != 0 {
				return c
			}
		}
		return cmpInt(len(x), len(y))
	}
	return strings.Compare(fmt.Sprint(a), fmt.Sprint(b))
}

func equal(a, b any) bool {
	// A string compares equal to a link with the same target, as people
	// write WHERE up = "Home" as often as WHERE up = [[Home]].
	if l, ok := a.(note.Link); ok {
		if s, ok := b.(string); ok {
			return linkMatchesName(l, s)
		}
	}
	if l, ok := b.(note.Link); ok {
		if s, ok := a.(string); ok {
			return linkMatchesName(l, s)
		}
	}
	return compare(a, b) == 0
}

func linkMatchesName(l note.Link, s string) bool {
	s = strings.TrimSuffix(s, ".md")
	p := strings.TrimSuffix(l.Path, ".md")
	return strings.EqualFold(p, s) || strings.EqualFold(baseName(p), s)
}

func cmpInt(a, b int) int { return cmpInt64(int64(a), int64(b)) }
func cmpInt64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func baseName(p string) string {
	p = strings.TrimSuffix(p, ".md")
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// display renders a value as text the way Dataview shows it in a table cell.
func display(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case bool:
		return strconv.FormatBool(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case string:
		return x
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 {
			return x.Format("January 02, 2006")
		}
		return x.Format("3:04 PM - January 02, 2006")
	case Duration:
		return displayDuration(x)
	case note.Link:
		if x.Display != "" {
			return x.Display
		}
		return baseName(x.Path)
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = display(e)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, len(keys))
		for i, k := range keys {
			parts[i] = k + ": " + display(x[k])
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

func displayDuration(d Duration) string {
	var parts []string
	add := func(n int64, unit string) {
		if n == 0 {
			return
		}
		if n != 1 {
			unit += "s"
		}
		parts = append(parts, fmt.Sprintf("%d %s", n, unit))
	}
	add(int64(d.Months/12), "year")
	add(int64(d.Months%12), "month")
	rest := d.D
	day := 24 * time.Hour
	add(int64(rest/(7*day)), "week")
	rest %= 7 * day
	add(int64(rest/day), "day")
	rest %= day
	add(int64(rest/time.Hour), "hour")
	rest %= time.Hour
	add(int64(rest/time.Minute), "minute")
	rest %= time.Minute
	add(int64(rest/time.Second), "second")
	if len(parts) == 0 {
		return "0 seconds"
	}
	return strings.Join(parts, ", ")
}
