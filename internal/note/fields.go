package note

import (
	"bytes"
	"encoding/json"
)

// Field is one key/value pair from frontmatter or an inline field.
type Field struct {
	Key   string
	Value any
}

// Fields keeps keys in file order. A slice instead of a map: notes have a
// handful of fields, and small maps cost several hundred bytes each, which
// dominated index memory (§V.5).
type Fields []Field

// Get returns the value for key, exact match first, then Dataview's
// canonical form (case-insensitive, spaces as dashes).
func (fs Fields) Get(key string) (any, bool) {
	for _, f := range fs {
		if f.Key == key {
			return f.Value, true
		}
	}
	ck := Canonical(key)
	for _, f := range fs {
		if Canonical(f.Key) == ck {
			return f.Value, true
		}
	}
	return nil, false
}

// add stores a value; a repeated key turns into a list, as in Dataview.
func (fs Fields) add(key string, v any) Fields {
	for i, f := range fs {
		if f.Key != key {
			continue
		}
		if l, ok := f.Value.([]any); ok {
			fs[i].Value = append(l, v)
		} else {
			fs[i].Value = []any{f.Value, v}
		}
		return fs
	}
	return append(fs, Field{key, v})
}

// MarshalJSON writes an object in file order.
func (fs Fields) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range fs {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(f.Key)
		v, err := json.Marshal(f.Value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}
