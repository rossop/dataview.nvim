// Package dql parses the v1 subset of Dataview Query Language into an AST.
// Pure core: no I/O, no clock — relative dates such as date(today) stay as
// calls and are evaluated by the executor.
package dql

import (
	"fmt"
	"strconv"
	"strings"
)

type Kind string

const (
	Table Kind = "TABLE"
	List  Kind = "LIST"
	Task  Kind = "TASK"
)

type Query struct {
	Kind      Kind
	WithoutID bool
	Columns   []Column // TABLE
	ListExpr  Expr     // LIST, optional
	From      Source   // nil = whole vault
	Commands  []Command
}

type Column struct {
	Expr Expr
	Name string // header: AS name, else the expression text
}

// Commands run in order, as in Dataview: WHERE then SORT then WHERE again is legal.
type Command interface{ command() }

type Where struct{ Cond Expr }
type Sort struct{ Keys []SortKey }
type Limit struct{ N int }

type SortKey struct {
	Expr Expr
	Desc bool
}

func (Where) command() {}
func (Sort) command()  {}
func (Limit) command() {}

// Sources select pages for FROM.
type Source interface{ source() }

type TagSource struct{ Tag string } // with leading '#'
type FolderSource struct{ Path string }
type LinkSource struct {
	Path     string
	Outgoing bool // outgoing([[x]]) = pages x links to; plain [[x]] = pages linking to x
}
type AndSource struct{ L, R Source }
type OrSource struct{ L, R Source }
type NotSource struct{ X Source }

func (TagSource) source()    {}
func (FolderSource) source() {}
func (LinkSource) source()   {}
func (AndSource) source()    {}
func (OrSource) source()     {}
func (NotSource) source()    {}

type Expr interface{ expr() }

// Lit holds nil, bool, float64, string or LinkLit.
type Lit struct{ Value any }
type LinkLit struct{ Path string }
type Ident struct{ Name string }
type FieldAccess struct {
	X    Expr
	Name string
}
type IndexExpr struct{ X, Key Expr }
type Call struct {
	Name string
	Args []Expr
}
type Unary struct {
	Op string // "-" or "!"
	X  Expr
}
type Binary struct {
	Op   string // = != < <= > >= + - * / % and or
	L, R Expr
}
type ListLit struct{ Elems []Expr }

func (Lit) expr()         {}
func (Ident) expr()       {}
func (FieldAccess) expr() {}
func (IndexExpr) expr()   {}
func (Call) expr()        {}
func (Unary) expr()       {}
func (Binary) expr()      {}
func (ListLit) expr()     {}

// Dump renders the AST as S-expressions for golden tests and debugging.
func Dump(q *Query) string {
	var b strings.Builder
	fmt.Fprintf(&b, "(%s", q.Kind)
	if q.WithoutID {
		b.WriteString(" without-id")
	}
	for _, c := range q.Columns {
		fmt.Fprintf(&b, "\n  (col %q %s)", c.Name, DumpExpr(c.Expr))
	}
	if q.ListExpr != nil {
		fmt.Fprintf(&b, "\n  (list %s)", DumpExpr(q.ListExpr))
	}
	if q.From != nil {
		fmt.Fprintf(&b, "\n  (from %s)", dumpSource(q.From))
	}
	for _, c := range q.Commands {
		switch c := c.(type) {
		case Where:
			fmt.Fprintf(&b, "\n  (where %s)", DumpExpr(c.Cond))
		case Sort:
			b.WriteString("\n  (sort")
			for _, k := range c.Keys {
				dir := "asc"
				if k.Desc {
					dir = "desc"
				}
				fmt.Fprintf(&b, " (%s %s)", DumpExpr(k.Expr), dir)
			}
			b.WriteString(")")
		case Limit:
			fmt.Fprintf(&b, "\n  (limit %d)", c.N)
		}
	}
	b.WriteString(")")
	return b.String()
}

func dumpSource(s Source) string {
	switch s := s.(type) {
	case TagSource:
		return s.Tag
	case FolderSource:
		return strconv.Quote(s.Path)
	case LinkSource:
		if s.Outgoing {
			return "(outgoing [[" + s.Path + "]])"
		}
		return "[[" + s.Path + "]]"
	case AndSource:
		return "(and " + dumpSource(s.L) + " " + dumpSource(s.R) + ")"
	case OrSource:
		return "(or " + dumpSource(s.L) + " " + dumpSource(s.R) + ")"
	case NotSource:
		return "(not " + dumpSource(s.X) + ")"
	}
	return "?"
}

func DumpExpr(e Expr) string {
	switch e := e.(type) {
	case Lit:
		switch v := e.Value.(type) {
		case nil:
			return "null"
		case string:
			return strconv.Quote(v)
		case LinkLit:
			return "[[" + v.Path + "]]"
		default:
			return fmt.Sprint(v)
		}
	case Ident:
		return e.Name
	case FieldAccess:
		return "(. " + DumpExpr(e.X) + " " + e.Name + ")"
	case IndexExpr:
		return "(index " + DumpExpr(e.X) + " " + DumpExpr(e.Key) + ")"
	case Call:
		parts := []string{e.Name}
		for _, a := range e.Args {
			parts = append(parts, DumpExpr(a))
		}
		return "(call " + strings.Join(parts, " ") + ")"
	case Unary:
		return "(" + e.Op + " " + DumpExpr(e.X) + ")"
	case Binary:
		return "(" + e.Op + " " + DumpExpr(e.L) + " " + DumpExpr(e.R) + ")"
	case ListLit:
		parts := []string{"list"}
		for _, x := range e.Elems {
			parts = append(parts, DumpExpr(x))
		}
		return "(" + strings.Join(parts, " ") + ")"
	}
	return "?"
}
