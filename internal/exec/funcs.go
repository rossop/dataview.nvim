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

func (e *executor) call(name string, a []any) (any, error) {
	arg := func(i int) any {
		if i < len(a) {
			return a[i]
		}
		return nil
	}
	str := func(i int) string {
		if s, ok := arg(i).(string); ok {
			return s
		}
		if arg(i) == nil {
			return ""
		}
		return display(arg(i))
	}
	need := func(n int) error {
		if len(a) < n {
			return fmt.Errorf("%s() needs %d argument(s), got %d", name, n, len(a))
		}
		return nil
	}

	switch name {
	case "date":
		if err := need(1); err != nil {
			return nil, err
		}
		return e.toDate(arg(0)), nil
	case "dur":
		if err := need(1); err != nil {
			return nil, err
		}
		switch v := arg(0).(type) {
		case Duration:
			return v, nil
		case string:
			if d, ok := parseDuration(v); ok {
				return d, nil
			}
		}
		return nil, nil
	case "length":
		switch v := arg(0).(type) {
		case []any:
			return float64(len(v)), nil
		case string:
			return float64(len([]rune(v))), nil
		case map[string]any:
			return float64(len(v)), nil
		}
		return 0.0, nil
	case "contains", "icontains", "econtains":
		if err := need(2); err != nil {
			return nil, err
		}
		return contains(arg(0), arg(1), name), nil
	case "startswith":
		return strings.HasPrefix(str(0), str(1)), nil
	case "endswith":
		return strings.HasSuffix(str(0), str(1)), nil
	case "lower":
		return strings.ToLower(str(0)), nil
	case "upper":
		return strings.ToUpper(str(0)), nil
	case "replace":
		if err := need(3); err != nil {
			return nil, err
		}
		return strings.ReplaceAll(str(0), str(1), str(2)), nil
	case "split":
		if err := need(2); err != nil {
			return nil, err
		}
		re, err := regexp.Compile(str(1))
		if err != nil {
			return nil, fmt.Errorf("split(): %v", err)
		}
		return strList(re.Split(str(0), -1)), nil
	case "regextest", "regexmatch":
		if err := need(2); err != nil {
			return nil, err
		}
		pat := str(0)
		if name == "regexmatch" {
			pat = "^(?:" + pat + ")$"
		}
		re, err := regexp.Compile(pat)
		if err != nil {
			return nil, fmt.Errorf("%s(): %v", name, err)
		}
		return re.MatchString(str(1)), nil
	case "join":
		sep := ", "
		if len(a) > 1 {
			sep = str(1)
		}
		l, ok := arg(0).([]any)
		if !ok {
			return display(arg(0)), nil
		}
		parts := make([]string, len(l))
		for i, v := range l {
			parts[i] = display(v)
		}
		return strings.Join(parts, sep), nil
	case "string":
		return display(arg(0)), nil
	case "number":
		switch v := arg(0).(type) {
		case float64:
			return v, nil
		case string:
			if m := regexp.MustCompile(`-?\d+(\.\d+)?`).FindString(v); m != "" {
				f, _ := strconv.ParseFloat(m, 64)
				return f, nil
			}
		}
		return nil, nil
	case "link":
		p := str(0)
		if l, ok := arg(0).(note.Link); ok {
			p = l.Path
		}
		l := e.resolveLink(note.Link{Path: p}, "")
		if len(a) > 1 {
			l.Display = str(1)
		}
		return l, nil
	case "default":
		if err := need(2); err != nil {
			return nil, err
		}
		if arg(0) == nil {
			return arg(1), nil
		}
		return arg(0), nil
	case "choice":
		if err := need(3); err != nil {
			return nil, err
		}
		if truthy(arg(0)) {
			return arg(1), nil
		}
		return arg(2), nil
	case "round":
		f, ok := arg(0).(float64)
		if !ok {
			return nil, nil
		}
		digits := 0.0
		if d, ok := arg(1).(float64); ok {
			digits = d
		}
		p := math.Pow(10, digits)
		return math.Round(f*p) / p, nil
	case "min", "max", "sum", "average":
		vals := a
		if len(a) == 1 {
			if l, ok := a[0].([]any); ok {
				vals = l
			}
		}
		return aggregate(name, vals), nil
	case "list", "array":
		return append([]any{}, a...), nil
	case "nonnull":
		var out []any
		for _, v := range listArg(a) {
			if v != nil {
				out = append(out, v)
			}
		}
		return out, nil
	case "any", "all":
		vals := listArg(a)
		for _, v := range vals {
			if truthy(v) == (name == "any") {
				return name == "any", nil
			}
		}
		return name == "all", nil
	case "reverse":
		l := append([]any{}, listArg(a)...)
		for i, j := 0, len(l)-1; i < j; i, j = i+1, j-1 {
			l[i], l[j] = l[j], l[i]
		}
		return l, nil
	case "sort":
		l := append([]any{}, listArg(a)...)
		sort.SliceStable(l, func(i, j int) bool { return compare(l[i], l[j]) < 0 })
		return l, nil
	case "flat":
		var out []any
		for _, v := range listArg(a) {
			if l, ok := v.([]any); ok {
				out = append(out, l...)
			} else {
				out = append(out, v)
			}
		}
		return out, nil
	case "typeof":
		return typeName(arg(0)), nil
	case "striptime":
		if t, ok := arg(0).(time.Time); ok {
			return truncDay(t), nil
		}
		return nil, nil
	case "dateformat":
		t, ok := e.toDate(arg(0)).(time.Time)
		if !ok {
			return nil, nil
		}
		return luxonFormat(t, str(1)), nil
	}
	return nil, fmt.Errorf("unknown function %s()", name)
}

// listArg accepts f(list) or f(a, b, c).
func listArg(a []any) []any {
	if len(a) == 1 {
		if l, ok := a[0].([]any); ok {
			return l
		}
	}
	return a
}

func contains(hay, needle any, mode string) bool {
	fold := mode == "icontains"
	switch h := hay.(type) {
	case []any:
		for _, v := range h {
			if mode == "econtains" || !isString(v) {
				if equal(v, needle) {
					return true
				}
				continue
			}
			// Dataview's contains on a list of strings matches substrings.
			if containsStr(v.(string), display(needle), fold) {
				return true
			}
		}
		return false
	case string:
		return containsStr(h, display(needle), fold)
	case map[string]any:
		_, ok := h[display(needle)]
		return ok
	case note.Link:
		return equal(h, needle)
	}
	return false
}

func isString(v any) bool { _, ok := v.(string); return ok }

func containsStr(s, sub string, fold bool) bool {
	if fold {
		return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
	}
	return strings.Contains(s, sub)
}

func aggregate(name string, vals []any) any {
	switch name {
	case "min", "max":
		var best any
		for _, v := range vals {
			if v == nil {
				continue
			}
			if best == nil || (name == "min" && compare(v, best) < 0) || (name == "max" && compare(v, best) > 0) {
				best = v
			}
		}
		return best
	}
	sum, n := 0.0, 0
	for _, v := range vals {
		if f, ok := v.(float64); ok {
			sum += f
			n++
		}
	}
	if name == "average" {
		if n == 0 {
			return nil
		}
		return sum / float64(n)
	}
	return sum
}

func (e *executor) toDate(v any) any {
	switch x := v.(type) {
	case time.Time:
		return x
	case note.Link:
		if n := e.ix.Get(x.Path); n != nil {
			return e.page(n).fileObject()["day"]
		}
		return nil
	case string:
		return e.parseDate(x)
	}
	return nil
}

// parseDate handles ISO dates and Dataview's relative keywords.
func (e *executor) parseDate(s string) any {
	now := e.wall(e.opt.Now)
	today := truncDay(now)
	endOfDay := func(t time.Time) time.Time { return t.Add(24*time.Hour - time.Millisecond) }
	sow := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7)) // ISO week starts Monday
	som := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	soy := time.Date(today.Year(), 1, 1, 0, 0, 0, 0, time.UTC)

	switch strings.ToLower(strings.TrimSpace(s)) {
	case "today":
		return today
	case "now":
		return now
	case "tomorrow":
		return today.AddDate(0, 0, 1)
	case "yesterday":
		return today.AddDate(0, 0, -1)
	case "sow":
		return sow
	case "eow":
		return endOfDay(sow.AddDate(0, 0, 6))
	case "som":
		return som
	case "eom":
		return endOfDay(som.AddDate(0, 1, -1))
	case "soy":
		return soy
	case "eoy":
		return endOfDay(soy.AddDate(1, 0, -1))
	}
	for _, layout := range []string{"2006-01-02", "2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02T15:04:05.000", "2006-01"} {
		if t, err := time.Parse(layout, strings.TrimSpace(s)); err == nil {
			return t
		}
	}
	return nil
}

// luxonFormat supports the common Luxon tokens used in dateformat().
func luxonFormat(t time.Time, f string) string {
	repl := []struct{ tok, layout string }{
		{"yyyy", "2006"}, {"yy", "06"},
		{"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
		{"dd", "02"}, {"d", "2"},
		{"EEEE", "Monday"}, {"EEE", "Mon"},
		{"HH", "15"}, {"hh", "03"}, {"h", "3"},
		{"mm", "04"}, {"ss", "05"}, {"a", "PM"},
	}
	var b strings.Builder
	for i := 0; i < len(f); {
		matched := false
		for _, r := range repl {
			if strings.HasPrefix(f[i:], r.tok) {
				b.WriteString(t.Format(r.layout))
				i += len(r.tok)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(f[i])
			i++
		}
	}
	return b.String()
}
