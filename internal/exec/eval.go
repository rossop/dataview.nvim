package exec

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/rossop/dataview.nvim/internal/dql"
	"github.com/rossop/dataview.nvim/internal/note"
)

func (e *executor) eval(x dql.Expr, s scope) (any, error) {
	switch x := x.(type) {
	case dql.Lit:
		if l, ok := x.Value.(dql.LinkLit); ok {
			return e.resolveLink(note.Link{Path: l.Path}, s.path()), nil
		}
		return x.Value, nil
	case dql.Ident:
		v, _ := s.lookup(x.Name)
		return v, nil
	case dql.FieldAccess:
		// file.x is computed field by field rather than via the whole object.
		if id, ok := x.X.(dql.Ident); ok && id.Name == "file" {
			if v, ok := s.page().fileField(x.Name); ok {
				return v, nil
			}
			return nil, nil
		}
		obj, err := e.eval(x.X, s)
		if err != nil {
			return nil, err
		}
		return e.field(obj, x.Name), nil
	case dql.IndexExpr:
		obj, err := e.eval(x.X, s)
		if err != nil {
			return nil, err
		}
		key, err := e.eval(x.Key, s)
		if err != nil {
			return nil, err
		}
		return e.index(obj, key), nil
	case dql.ListLit:
		out := make([]any, len(x.Elems))
		for i, el := range x.Elems {
			v, err := e.eval(el, s)
			if err != nil {
				return nil, err
			}
			out[i] = v
		}
		return out, nil
	case dql.Unary:
		v, err := e.eval(x.X, s)
		if err != nil {
			return nil, err
		}
		if x.Op == "!" {
			return !truthy(v), nil
		}
		switch n := v.(type) {
		case nil:
			return nil, nil
		case float64:
			return -n, nil
		case Duration:
			return Duration{-n.Months, -n.D}, nil
		}
		return nil, fmt.Errorf("cannot negate %s", typeName(v))
	case dql.Binary:
		return e.binary(x, s)
	case dql.Call:
		args := make([]any, len(x.Args))
		for i, a := range x.Args {
			v, err := e.eval(a, s)
			if err != nil {
				return nil, err
			}
			args[i] = v
		}
		return e.call(x.Name, args)
	}
	return nil, fmt.Errorf("unsupported expression %T", x)
}

func (e *executor) field(obj any, name string) any {
	switch o := obj.(type) {
	case map[string]any:
		if v, ok := o[name]; ok {
			return v
		}
		c := note.Canonical(name)
		for k, v := range o {
			if note.Canonical(k) == c {
				return v
			}
		}
	case note.Link:
		// [[Page]].field reads the linked page.
		if n := e.ix.Get(o.Path); n != nil {
			return e.field(e.pageObject(n), name)
		}
	case []any:
		// Dataview "swizzles": list.field maps over the elements.
		out := make([]any, len(o))
		for i, el := range o {
			out[i] = e.field(el, name)
		}
		return out
	case time.Time:
		return dateField(o, name)
	}
	return nil
}

func dateField(t time.Time, name string) any {
	switch name {
	case "year":
		return float64(t.Year())
	case "month":
		return float64(t.Month())
	case "day":
		return float64(t.Day())
	case "hour":
		return float64(t.Hour())
	case "minute":
		return float64(t.Minute())
	case "second":
		return float64(t.Second())
	case "weekday":
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		return float64(wd)
	case "week":
		_, w := t.ISOWeek()
		return float64(w)
	}
	return nil
}

func (e *executor) index(obj, key any) any {
	switch o := obj.(type) {
	case []any:
		if f, ok := key.(float64); ok {
			i := int(f)
			if i < 0 {
				i += len(o)
			}
			if i >= 0 && i < len(o) {
				return o[i]
			}
		}
	case map[string]any, note.Link:
		if s, ok := key.(string); ok {
			return e.field(o, s)
		}
	case string:
		if f, ok := key.(float64); ok {
			r := []rune(o)
			if i := int(f); i >= 0 && i < len(r) {
				return string(r[i])
			}
		}
	}
	return nil
}

func (e *executor) binary(x dql.Binary, s scope) (any, error) {
	l, err := e.eval(x.L, s)
	if err != nil {
		return nil, err
	}
	switch x.Op {
	case "and":
		if !truthy(l) {
			return false, nil
		}
		r, err := e.eval(x.R, s)
		return truthy(r), err
	case "or":
		if truthy(l) {
			return true, nil
		}
		r, err := e.eval(x.R, s)
		return truthy(r), err
	}
	r, err := e.eval(x.R, s)
	if err != nil {
		return nil, err
	}
	switch x.Op {
	case "=":
		return equal(l, r), nil
	case "!=":
		return !equal(l, r), nil
	case "<":
		return compare(l, r) < 0, nil
	case "<=":
		return compare(l, r) <= 0, nil
	case ">":
		return compare(l, r) > 0, nil
	case ">=":
		return compare(l, r) >= 0, nil
	}
	return arith(x.Op, l, r)
}

func arith(op string, l, r any) (any, error) {
	if l == nil || r == nil {
		if op == "+" {
			if s, ok := l.(string); ok {
				return s + display(r), nil
			}
			if s, ok := r.(string); ok {
				return display(l) + s, nil
			}
		}
		return nil, nil
	}
	switch a := l.(type) {
	case float64:
		switch b := r.(type) {
		case float64:
			switch op {
			case "+":
				return a + b, nil
			case "-":
				return a - b, nil
			case "*":
				return a * b, nil
			case "/":
				if b == 0 {
					return nil, nil
				}
				return a / b, nil
			case "%":
				if b == 0 {
					return nil, nil
				}
				return math.Mod(a, b), nil
			}
		case Duration:
			if op == "*" {
				return scaleDur(b, a), nil
			}
		}
	case time.Time:
		switch b := r.(type) {
		case Duration:
			switch op {
			case "+":
				return addMonths(a, b.Months).Add(b.D), nil
			case "-":
				return addMonths(a, -b.Months).Add(-b.D), nil
			}
		case time.Time:
			if op == "-" {
				return Duration{D: a.Sub(b)}, nil
			}
		}
	case Duration:
		switch b := r.(type) {
		case Duration:
			switch op {
			case "+":
				return Duration{a.Months + b.Months, a.D + b.D}, nil
			case "-":
				return Duration{a.Months - b.Months, a.D - b.D}, nil
			}
		case time.Time:
			if op == "+" {
				return addMonths(b, a.Months).Add(a.D), nil
			}
		case float64:
			switch op {
			case "*":
				return scaleDur(a, b), nil
			case "/":
				if b == 0 {
					return nil, nil
				}
				return scaleDur(a, 1/b), nil
			}
		}
	case []any:
		if b, ok := r.([]any); ok && op == "+" {
			return append(append([]any{}, a...), b...), nil
		}
	}
	if op == "+" {
		_, ls := l.(string)
		_, rs := r.(string)
		if ls || rs {
			return display(l) + display(r), nil
		}
	}
	return nil, fmt.Errorf("no '%s' for %s and %s", op, typeName(l), typeName(r))
}

// addMonths clamps to the last day of the target month like Luxon, so
// Jan 31 + 1 month is Feb 28, not Mar 3 as time.AddDate would give.
func addMonths(t time.Time, months int) time.Time {
	if months == 0 {
		return t
	}
	first := time.Date(t.Year(), t.Month(), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location())
	target := first.AddDate(0, months, 0)
	last := target.AddDate(0, 1, -1).Day()
	day := min(t.Day(), last)
	return target.AddDate(0, 0, day-1)
}

func scaleDur(d Duration, f float64) Duration {
	return Duration{int(math.Round(float64(d.Months) * f)), time.Duration(float64(d.D) * f)}
}

func typeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case time.Time:
		return "date"
	case Duration:
		return "duration"
	case note.Link:
		return "link"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	}
	return strings.ToLower(fmt.Sprintf("%T", v))
}
