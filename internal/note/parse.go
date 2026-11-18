package note

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const fieldKey = `[\p{L}\p{N}_][\p{L}\p{N}_\- /]*?`

var (
	// "key:: value" on its own line, optionally a list item or bolded key.
	lineFieldRe = regexp.MustCompile(`^\s*(?:[-*+]\s+|\d+[.)]\s+)?(?:\*\*|__)?(` + fieldKey + `)(?:\*\*|__)?::\s*(.*)$`)
	// "[key:: value]" and "(key:: value)" anywhere in a line.
	squareFieldRe = regexp.MustCompile(`\[(` + fieldKey + `)::\s*([^\]]*)\]`)
	parenFieldRe  = regexp.MustCompile(`\((` + fieldKey + `)::\s*([^)]*)\)`)

	tagRe      = regexp.MustCompile(`(?:^|[\s(])#([\p{L}\p{N}_/\-]+)`)
	digitsRe   = regexp.MustCompile(`^[\d/]+$`)
	wikiLinkRe = regexp.MustCompile(`(!?)\[\[([^\[\]]+?)\]\]`)
	mdLinkRe   = regexp.MustCompile(`(!?)\[([^\]]*)\]\(([^()\s]+)\)`)
	schemeRe   = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.\-]*:`)
	taskRe     = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+\[(.)\]\s?(.*)$`)
	inlineCode = regexp.MustCompile("`[^`]*`")
)

// Parse extracts metadata from one note. relPath is vault-relative with
// forward slashes. Warnings describe recoverable problems (§V.3); Parse never
// fails, so one bad note cannot abort a scan.
func Parse(relPath string, src []byte) (*Note, []string) {
	n := &Note{Path: relPath, Size: int64(len(src))}
	var warns []string

	lines := strings.Split(strings.ReplaceAll(string(src), "\r\n", "\n"), "\n")
	bodyStart := 0
	if fm, end, ok := splitFrontmatter(lines); ok {
		bodyStart = end
		if fields, err := parseFrontmatter(fm); err != nil {
			warns = append(warns, fmt.Sprintf("%s: invalid frontmatter ignored: %v", relPath, firstLine(err.Error())))
		} else {
			n.Frontmatter = fields
			n.addFrontmatterMeta()
		}
	}

	inFence := false
	fence := ""
	for i := bodyStart; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if marker := fenceMarker(trimmed); marker != "" {
			switch {
			case !inFence:
				inFence, fence = true, marker
			case strings.HasPrefix(trimmed, fence):
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		n.scanLine(i, stripCode(line))
	}
	return n, warns
}

// parseFrontmatter decodes via yaml.Node so keys keep their file order.
func parseFrontmatter(src string) (Fields, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("frontmatter is not a mapping")
	}
	out := make(Fields, 0, len(m.Content)/2)
	for i := 0; i+1 < len(m.Content); i += 2 {
		var v any
		if err := m.Content[i+1].Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, Field{m.Content[i].Value, normalize(v)})
	}
	return out, nil
}

func splitFrontmatter(lines []string) (string, int, bool) {
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return "", 0, false
	}
	for i := 1; i < len(lines); i++ {
		if l := strings.TrimRight(lines[i], " \t"); l == "---" || l == "..." {
			return strings.Join(lines[1:i], "\n"), i + 1, true
		}
	}
	return "", 0, false
}

func fenceMarker(trimmed string) string {
	for _, m := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, m) {
			return m
		}
	}
	return ""
}

// stripCode blanks inline code spans so tags and links inside them are ignored
// while byte offsets stay meaningful.
func stripCode(line string) string {
	return inlineCode.ReplaceAllStringFunc(line, func(s string) string {
		return strings.Repeat(" ", len(s))
	})
}

// scanLine collects metadata from one body line. Stored strings are cloned so
// a note never pins its source text, whatever the line came from (§V.4).
func (n *Note) scanLine(lineNo int, line string) {
	tags := extractTags(line)
	links := n.extractLinks(line)
	fields := extractFields(line)

	n.Tags = appendUnique(n.Tags, tags...)
	n.Links = append(n.Links, links...)
	for _, f := range fields {
		n.Fields = n.Fields.add(f.key, f.value)
	}

	if m := taskRe.FindStringSubmatch(line); m != nil {
		t := Task{
			Line:      lineNo,
			Status:    strings.Clone(m[1]),
			Completed: m[1] == "x" || m[1] == "X",
			Text:      strings.Clone(strings.TrimSpace(m[2])),
			Tags:      tags,
			Links:     links,
		}
		for _, f := range fields {
			t.Fields = t.Fields.add(f.key, f.value)
		}
		n.Tasks = append(n.Tasks, t)
	}
}

type field struct {
	key   string
	value any
}

func extractFields(line string) []field {
	var out []field
	for _, re := range []*regexp.Regexp{squareFieldRe, parenFieldRe} {
		for _, m := range re.FindAllStringSubmatch(line, -1) {
			out = append(out, field{strings.Clone(strings.TrimSpace(m[1])), ParseValue(m[2])})
		}
	}
	if len(out) > 0 {
		return out
	}
	// A task line is never a whole-line field: "- [ ] a:: b" belongs to the task text.
	if taskRe.MatchString(line) {
		return nil
	}
	if m := lineFieldRe.FindStringSubmatch(line); m != nil {
		out = append(out, field{strings.Clone(strings.TrimSpace(m[1])), ParseValue(m[2])})
	}
	return out
}

func extractTags(line string) []string {
	var out []string
	for _, m := range tagRe.FindAllStringSubmatch(line, -1) {
		tag := strings.TrimRight(m[1], "/")
		// Obsidian requires at least one non-numeric character.
		if tag == "" || digitsRe.MatchString(tag) {
			continue
		}
		out = appendUnique(out, "#"+tag) // concatenation copies, so the body is not retained
	}
	return out
}

func (n *Note) extractLinks(line string) []Link {
	var out []Link
	for _, m := range wikiLinkRe.FindAllStringSubmatch(line, -1) {
		out = append(out, newWikiLink(m[2], m[1] == "!"))
	}
	for _, m := range mdLinkRe.FindAllStringSubmatch(line, -1) {
		if l, ok := n.mdLink(m[3], m[2], m[1] == "!"); ok {
			out = append(out, l)
		}
	}
	return out
}

func newWikiLink(inner string, embed bool) Link {
	target, display, _ := strings.Cut(inner, "|")
	target, sub, _ := strings.Cut(target, "#")
	return Link{
		Path:    strings.Clone(strings.TrimSuffix(strings.TrimSpace(target), ".md")),
		Subpath: strings.Clone(strings.TrimSpace(sub)),
		Display: strings.Clone(strings.TrimSpace(display)),
		Embed:   embed,
	}
}

// mdLink keeps only links to other notes; URLs and in-page anchors are not
// vault links.
func (n *Note) mdLink(target, display string, embed bool) (Link, bool) {
	if schemeRe.MatchString(target) || strings.HasPrefix(target, "#") {
		return Link{}, false
	}
	target, sub, _ := strings.Cut(target, "#")
	if dec, err := url.PathUnescape(target); err == nil {
		target = dec
	}
	if ext := path.Ext(target); ext != "" && ext != ".md" {
		return Link{}, false
	}
	p := strings.TrimSuffix(path.Join(n.Folder(), target), ".md")
	if strings.HasPrefix(p, "../") {
		return Link{}, false
	}
	return Link{Path: strings.Clone(p), Subpath: strings.Clone(sub), Display: strings.Clone(display), Embed: embed}, true
}

// addFrontmatterMeta pulls tags, aliases and links out of frontmatter.
func (n *Note) addFrontmatterMeta() {
	for _, key := range []string{"tags", "tag"} {
		v, _ := n.Frontmatter.Get(key)
		for _, t := range stringList(v) {
			t = strings.TrimPrefix(strings.TrimSpace(t), "#")
			if t != "" {
				n.Tags = appendUnique(n.Tags, "#"+t)
			}
		}
	}
	for _, key := range []string{"aliases", "alias"} {
		v, _ := n.Frontmatter.Get(key)
		n.Aliases = append(n.Aliases, stringList(v)...)
	}
	for _, f := range n.Frontmatter {
		n.Links = append(n.Links, collectLinks(f.Value)...)
	}
}

// stringList accepts YAML lists or comma/space separated strings.
func stringList(v any) []string {
	switch x := v.(type) {
	case string:
		return strings.FieldsFunc(x, func(r rune) bool { return r == ',' || r == ' ' })
	case []any:
		var out []string
		for _, e := range x {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func collectLinks(v any) []Link {
	switch x := v.(type) {
	case Link:
		return []Link{x}
	case []any:
		var out []Link
		for _, e := range x {
			out = append(out, collectLinks(e)...)
		}
		return out
	case map[string]any:
		// Sorted so link order is deterministic across runs.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []Link
		for _, k := range keys {
			out = append(out, collectLinks(x[k])...)
		}
		return out
	}
	return nil
}

func appendUnique(dst []string, vals ...string) []string {
	for _, v := range vals {
		found := false
		for _, d := range dst {
			if d == v {
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, v)
		}
	}
	return dst
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
