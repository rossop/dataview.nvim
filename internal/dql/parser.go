package dql

import (
	"regexp"
	"strconv"
	"strings"
)

// Parse parses one DQL query (the text inside a ```dataview block).
func Parse(src string) (*Query, error) {
	p := &parser{lx: lexer{src: src}}
	if err := p.advance(); err != nil {
		return nil, err
	}
	q, err := p.query()
	if err != nil {
		return nil, err
	}
	return q, nil
}

type parser struct {
	lx      lexer
	tok     token
	prevEnd int
}

func (p *parser) advance() error {
	p.prevEnd = p.tok.end
	t, err := p.lx.next()
	if err != nil {
		return err
	}
	p.tok = t
	return nil
}

func (p *parser) errf(format string, args ...any) error {
	return p.lx.errAt(p.tok.pos, format, args...)
}

func (p *parser) isKeyword(words ...string) bool {
	if p.tok.kind != tIdent {
		return false
	}
	for _, w := range words {
		if strings.EqualFold(p.tok.text, w) {
			return true
		}
	}
	return false
}

func (p *parser) isOp(ops ...string) bool {
	if p.tok.kind != tOp {
		return false
	}
	for _, o := range ops {
		if p.tok.text == o {
			return true
		}
	}
	return false
}

func (p *parser) expectOp(op string) error {
	if !p.isOp(op) {
		return p.errf("expected %q, found %s", op, p.describe())
	}
	return p.advance()
}

func (p *parser) describe() string {
	if p.tok.kind == tEOF {
		return "end of query"
	}
	return strconv.Quote(p.lx.src[p.tok.pos:p.tok.end])
}

var clauseWords = []string{"FROM", "WHERE", "SORT", "LIMIT", "GROUP", "FLATTEN"}

func (p *parser) atClauseOrEOF() bool {
	return p.tok.kind == tEOF || p.isKeyword(clauseWords...)
}

func (p *parser) query() (*Query, error) {
	q := &Query{}
	switch {
	case p.isKeyword("TABLE"):
		q.Kind = Table
	case p.isKeyword("LIST"):
		q.Kind = List
	case p.isKeyword("TASK"):
		q.Kind = Task
	case p.isKeyword("CALENDAR"):
		return nil, p.errf("CALENDAR queries are not supported yet")
	default:
		return nil, p.errf("expected TABLE, LIST or TASK, found %s", p.describe())
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	if q.Kind != Task && p.isKeyword("WITHOUT") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		if !p.isKeyword("ID") {
			return nil, p.errf("expected ID after WITHOUT")
		}
		q.WithoutID = true
		if err := p.advance(); err != nil {
			return nil, err
		}
	}

	switch {
	case q.Kind == Table && !p.atClauseOrEOF():
		cols, err := p.columns()
		if err != nil {
			return nil, err
		}
		q.Columns = cols
	case q.Kind == List && !p.atClauseOrEOF():
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		q.ListExpr = e
	}

	if p.isKeyword("FROM") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		s, err := p.sourceOr()
		if err != nil {
			return nil, err
		}
		q.From = s
	}

	for p.tok.kind != tEOF {
		switch {
		case p.isKeyword("WHERE"):
			if err := p.advance(); err != nil {
				return nil, err
			}
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			q.Commands = append(q.Commands, Where{e})
		case p.isKeyword("SORT"):
			if err := p.advance(); err != nil {
				return nil, err
			}
			keys, err := p.sortKeys()
			if err != nil {
				return nil, err
			}
			q.Commands = append(q.Commands, Sort{keys})
		case p.isKeyword("LIMIT"):
			if err := p.advance(); err != nil {
				return nil, err
			}
			if p.tok.kind != tNumber {
				return nil, p.errf("LIMIT needs a number")
			}
			n, err := strconv.Atoi(p.tok.text)
			if err != nil {
				return nil, p.errf("LIMIT needs a whole number")
			}
			q.Commands = append(q.Commands, Limit{n})
			if err := p.advance(); err != nil {
				return nil, err
			}
		case p.isKeyword("GROUP", "FLATTEN"):
			return nil, p.errf("%s is not supported yet", strings.ToUpper(p.tok.text))
		case p.isKeyword("FROM"):
			return nil, p.errf("FROM must come before WHERE, SORT and LIMIT")
		default:
			return nil, p.errf("unexpected %s", p.describe())
		}
	}
	return q, nil
}

func (p *parser) columns() ([]Column, error) {
	var cols []Column
	for {
		start := p.tok.pos
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		name := strings.TrimSpace(p.lx.src[start:p.prevEnd])
		if p.isKeyword("AS") {
			if err := p.advance(); err != nil {
				return nil, err
			}
			if p.tok.kind != tIdent && p.tok.kind != tString {
				return nil, p.errf("expected a column name after AS")
			}
			name = p.tok.text
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
		cols = append(cols, Column{e, name})
		if !p.isOp(",") {
			return cols, nil
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
}

func (p *parser) sortKeys() ([]SortKey, error) {
	var keys []SortKey
	for {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		k := SortKey{Expr: e}
		if p.isKeyword("ASC", "ASCENDING", "DESC", "DESCENDING") {
			k.Desc = strings.HasPrefix(strings.ToUpper(p.tok.text), "DESC")
			if err := p.advance(); err != nil {
				return nil, err
			}
		}
		keys = append(keys, k)
		if !p.isOp(",") {
			return keys, nil
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
}

// ---- FROM sources ----

func (p *parser) sourceOr() (Source, error) {
	l, err := p.sourceAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("OR") || p.isOp("|") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.sourceAnd()
		if err != nil {
			return nil, err
		}
		l = OrSource{l, r}
	}
	return l, nil
}

func (p *parser) sourceAnd() (Source, error) {
	l, err := p.sourceUnary()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("AND") || p.isOp("&") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.sourceUnary()
		if err != nil {
			return nil, err
		}
		l = AndSource{l, r}
	}
	return l, nil
}

func (p *parser) sourceUnary() (Source, error) {
	t := p.tok
	switch {
	case p.isOp("-", "!"):
		if err := p.advance(); err != nil {
			return nil, err
		}
		x, err := p.sourceUnary()
		if err != nil {
			return nil, err
		}
		return NotSource{x}, nil
	case p.isOp("("):
		if err := p.advance(); err != nil {
			return nil, err
		}
		s, err := p.sourceOr()
		if err != nil {
			return nil, err
		}
		return s, p.expectOp(")")
	case t.kind == tTag:
		return TagSource{t.text}, p.advance()
	case t.kind == tString:
		return FolderSource{strings.Trim(t.text, "/")}, p.advance()
	case t.kind == tLink:
		return LinkSource{Path: t.text}, p.advance()
	case p.isKeyword("outgoing"):
		if err := p.advance(); err != nil {
			return nil, err
		}
		if err := p.expectOp("("); err != nil {
			return nil, err
		}
		if p.tok.kind != tLink {
			return nil, p.errf("outgoing() needs a [[link]]")
		}
		path := p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		return LinkSource{Path: path, Outgoing: true}, p.expectOp(")")
	}
	return nil, p.errf("expected a source (#tag, \"folder\" or [[link]]), found %s", p.describe())
}

// ---- expressions ----

func (p *parser) expr() (Expr, error) { return p.or() }

func (p *parser) or() (Expr, error) {
	l, err := p.and()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("or") || p.isOp("|") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.and()
		if err != nil {
			return nil, err
		}
		l = Binary{"or", l, r}
	}
	return l, nil
}

func (p *parser) and() (Expr, error) {
	l, err := p.cmp()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("and") || p.isOp("&") {
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.cmp()
		if err != nil {
			return nil, err
		}
		l = Binary{"and", l, r}
	}
	return l, nil
}

func (p *parser) cmp() (Expr, error) {
	l, err := p.add()
	if err != nil {
		return nil, err
	}
	if p.isOp("=", "!=", "<", "<=", ">", ">=") {
		op := p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.add()
		if err != nil {
			return nil, err
		}
		return Binary{op, l, r}, nil
	}
	return l, nil
}

func (p *parser) add() (Expr, error) {
	l, err := p.mul()
	if err != nil {
		return nil, err
	}
	for p.isOp("+", "-") {
		op := p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.mul()
		if err != nil {
			return nil, err
		}
		l = Binary{op, l, r}
	}
	return l, nil
}

func (p *parser) mul() (Expr, error) {
	l, err := p.unary()
	if err != nil {
		return nil, err
	}
	for p.isOp("*", "/", "%") {
		op := p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		r, err := p.unary()
		if err != nil {
			return nil, err
		}
		l = Binary{op, l, r}
	}
	return l, nil
}

func (p *parser) unary() (Expr, error) {
	if p.isOp("-", "!") {
		op := p.tok.text
		if err := p.advance(); err != nil {
			return nil, err
		}
		x, err := p.unary()
		if err != nil {
			return nil, err
		}
		return Unary{op, x}, nil
	}
	return p.postfix()
}

func (p *parser) postfix() (Expr, error) {
	x, err := p.primary()
	if err != nil {
		return nil, err
	}
	for {
		switch {
		case p.isOp("."):
			if err := p.advance(); err != nil {
				return nil, err
			}
			if p.tok.kind != tIdent {
				return nil, p.errf("expected a field name after '.'")
			}
			x = FieldAccess{x, p.tok.text}
			if err := p.advance(); err != nil {
				return nil, err
			}
		case p.isOp("["):
			if err := p.advance(); err != nil {
				return nil, err
			}
			k, err := p.expr()
			if err != nil {
				return nil, err
			}
			if err := p.expectOp("]"); err != nil {
				return nil, err
			}
			x = IndexExpr{x, k}
		default:
			return x, nil
		}
	}
}

// Unquoted literal arguments Dataview accepts: date(today), date(2026-01-31),
// dur(1 week), dur(2 days, 3 hours).
var (
	rawDateArg = regexp.MustCompile(`^(?i:today|now|tomorrow|yesterday|sow|eow|som|eom|soy|eoy|\d{4}-\d{2}(-\d{2}(T\d{2}:\d{2}(:\d{2}(\.\d+)?)?)?)?)$`)
	rawDurArg  = regexp.MustCompile(`^(?i:\d+(\.\d+)?\s*[a-z]+(\s*,?\s*\d+(\.\d+)?\s*[a-z]+)*)$`)
)

func (p *parser) primary() (Expr, error) {
	t := p.tok
	switch t.kind {
	case tNumber:
		f, err := strconv.ParseFloat(t.text, 64)
		if err != nil {
			return nil, p.errf("bad number %q", t.text)
		}
		return Lit{f}, p.advance()
	case tString:
		return Lit{t.text}, p.advance()
	case tLink:
		return Lit{LinkLit{t.text}}, p.advance()
	case tIdent:
		switch strings.ToLower(t.text) {
		case "true":
			return Lit{true}, p.advance()
		case "false":
			return Lit{false}, p.advance()
		case "null":
			return Lit{nil}, p.advance()
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
		if !p.isOp("(") {
			return Ident{t.text}, nil
		}
		return p.call(t.text)
	case tOp:
		switch t.text {
		case "(":
			if err := p.advance(); err != nil {
				return nil, err
			}
			e, err := p.expr()
			if err != nil {
				return nil, err
			}
			return e, p.expectOp(")")
		case "[":
			return p.list()
		}
	}
	return nil, p.errf("expected an expression, found %s", p.describe())
}

// call parses arguments; p.tok is the opening '('.
func (p *parser) call(name string) (Expr, error) {
	lname := strings.ToLower(name)
	if lname == "date" || lname == "dur" {
		open := p.tok.end
		if close := strings.IndexByte(p.lx.src[open:], ')'); close >= 0 {
			raw := strings.TrimSpace(p.lx.src[open : open+close])
			if (lname == "date" && rawDateArg.MatchString(raw)) || (lname == "dur" && rawDurArg.MatchString(raw)) {
				p.lx.pos = open + close + 1
				p.tok.end = p.lx.pos
				return Call{lname, []Expr{Lit{raw}}}, p.advance()
			}
		}
	}
	if err := p.advance(); err != nil {
		return nil, err
	}
	var args []Expr
	for !p.isOp(")") {
		a, err := p.expr()
		if err != nil {
			return nil, err
		}
		args = append(args, a)
		if !p.isOp(",") {
			break
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	return Call{lname, args}, p.expectOp(")")
}

func (p *parser) list() (Expr, error) {
	if err := p.advance(); err != nil {
		return nil, err
	}
	var elems []Expr
	for !p.isOp("]") {
		e, err := p.expr()
		if err != nil {
			return nil, err
		}
		elems = append(elems, e)
		if !p.isOp(",") {
			break
		}
		if err := p.advance(); err != nil {
			return nil, err
		}
	}
	return ListLit{elems}, p.expectOp("]")
}
