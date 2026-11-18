package dql

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokKind int

const (
	tEOF tokKind = iota
	tIdent
	tNumber
	tString
	tTag
	tLink
	tOp
)

type token struct {
	kind     tokKind
	text     string // ident/op text, decoded string, tag with '#', link inner path
	pos, end int    // byte offsets in source
}

// Error carries a 1-based line and column so editors can point at it.
type Error struct {
	Line, Col int
	Msg       string
}

func (e *Error) Error() string { return fmt.Sprintf("%d:%d: %s", e.Line, e.Col, e.Msg) }

type lexer struct {
	src string
	pos int
}

func (l *lexer) errAt(pos int, format string, args ...any) *Error {
	line := 1 + strings.Count(l.src[:pos], "\n")
	col := pos - strings.LastIndex(l.src[:pos], "\n")
	return &Error{line, col, fmt.Sprintf(format, args...)}
}

func isIdentStart(r rune) bool { return unicode.IsLetter(r) || r == '_' }

// Dataview identifiers may contain '-', so "due-date" is one name and
// subtraction needs spaces: "a - b".
func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

func isTagPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/'
}

func (l *lexer) next() (token, error) {
	for l.pos < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.pos:])
		if !unicode.IsSpace(r) {
			break
		}
		l.pos += w
	}
	start := l.pos
	if start >= len(l.src) {
		return token{kind: tEOF, pos: start, end: start}, nil
	}
	r, w := utf8.DecodeRuneInString(l.src[start:])
	s := l.src[start:]

	switch {
	case isIdentStart(r):
		l.pos += w
		for l.pos < len(l.src) {
			r, w := utf8.DecodeRuneInString(l.src[l.pos:])
			if !isIdentPart(r) {
				break
			}
			l.pos += w
		}
		// A trailing '-' belongs to a following operand ("x- 1" is rare, "x -1" common).
		for l.pos > start+1 && l.src[l.pos-1] == '-' {
			l.pos--
		}
		return token{tIdent, l.src[start:l.pos], start, l.pos}, nil

	case unicode.IsDigit(r):
		for l.pos < len(l.src) && (isDigit(l.src[l.pos]) || (l.src[l.pos] == '.' && l.pos+1 < len(l.src) && isDigit(l.src[l.pos+1]))) {
			l.pos++
		}
		return token{tNumber, l.src[start:l.pos], start, l.pos}, nil

	case r == '"':
		var b strings.Builder
		l.pos++
		for {
			if l.pos >= len(l.src) {
				return token{}, l.errAt(start, "unterminated string")
			}
			c := l.src[l.pos]
			if c == '"' {
				l.pos++
				break
			}
			if c == '\\' && l.pos+1 < len(l.src) {
				l.pos++
				switch e := l.src[l.pos]; e {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(e)
				}
				l.pos++
				continue
			}
			b.WriteByte(c)
			l.pos++
		}
		return token{tString, b.String(), start, l.pos}, nil

	case r == '#':
		l.pos++
		for l.pos < len(l.src) {
			r, w := utf8.DecodeRuneInString(l.src[l.pos:])
			if !isTagPart(r) {
				break
			}
			l.pos += w
		}
		if l.pos == start+1 {
			return token{}, l.errAt(start, "empty tag")
		}
		return token{tTag, l.src[start:l.pos], start, l.pos}, nil

	case strings.HasPrefix(s, "[["):
		end := strings.Index(s, "]]")
		if end < 0 {
			return token{}, l.errAt(start, "unterminated link")
		}
		l.pos += end + 2
		inner, _, _ := strings.Cut(s[2:end], "|")
		return token{tLink, strings.TrimSpace(inner), start, l.pos}, nil
	}

	for _, op := range []string{"!=", "<=", ">=", "(", ")", "[", "]", ",", ".", "=", "<", ">", "+", "-", "*", "/", "%", "!", "&", "|"} {
		if strings.HasPrefix(s, op) {
			l.pos += len(op)
			return token{tOp, op, start, l.pos}, nil
		}
	}
	return token{}, l.errAt(start, "unexpected character %q", r)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
