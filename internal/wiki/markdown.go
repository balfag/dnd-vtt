package wiki

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

// Linker resolves links while rendering Markdown.
type Linker interface {
	Page(id string) (href string, ok bool) // relative link to the page file; ok=false: plain text
	Asset(id string) string                // image URL or relative path
	Label(r doc.Ref, stored string) string // current name of a mention target
}

type mdNode struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs"`
	Content []*mdNode      `json:"content"`
	Marks   []mdMark       `json:"marks"`
	Text    string         `json:"text"`
}

type mdMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

func (n *mdNode) str(key string) string {
	s, _ := n.Attrs[key].(string)
	return s
}

func (n *mdNode) num(key string, def int) int {
	if f, ok := n.Attrs[key].(float64); ok {
		return int(f)
	}
	return def
}

// ToMarkdown renders a normalized wiki document (doc.Validate output).
func ToMarkdown(content []byte, l Linker) (string, error) {
	var root mdNode
	if err := json.Unmarshal(content, &root); err != nil {
		return "", fmt.Errorf("markdown: parse document: %w", err)
	}
	if root.Type != "doc" {
		return "", fmt.Errorf("markdown: the root is %q, not a doc", root.Type)
	}
	r := &mdRenderer{l: l}
	return r.blocks(root.Content)
}

type mdRenderer struct {
	l       Linker
	inTable bool // a table cell is rendered: | must be escaped
}

// blocks renders sibling blocks separated by an empty line; empty blocks are dropped.
func (r *mdRenderer) blocks(nodes []*mdNode) (string, error) {
	var parts []string
	for _, n := range nodes {
		s, err := r.block(n)
		if err != nil {
			return "", err
		}
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n"), nil
}

func isList(n *mdNode) bool {
	return n.Type == "bulletList" || n.Type == "orderedList" || n.Type == "taskList"
}

// itemBlocks is blocks for the content of a list item: a nested list follows
// its paragraph without an empty line.
func (r *mdRenderer) itemBlocks(nodes []*mdNode) (string, error) {
	var b strings.Builder
	for i, n := range nodes {
		s, err := r.block(n)
		if err != nil {
			return "", err
		}
		if s == "" {
			continue
		}
		if b.Len() > 0 {
			if isList(n) && i > 0 && nodes[i-1].Type == "paragraph" {
				b.WriteString("\n")
			} else {
				b.WriteString("\n\n")
			}
		}
		b.WriteString(s)
	}
	return b.String(), nil
}

// prefixLines puts first before the first line and rest before the others;
// empty lines get the trimmed prefix so no trailing spaces appear.
func prefixLines(s, first, rest string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		p := rest
		if i == 0 {
			p = first
		}
		if ln == "" {
			lines[i] = strings.TrimRight(p, " ")
			continue
		}
		lines[i] = p + ln
	}
	return strings.Join(lines, "\n")
}

func (r *mdRenderer) block(n *mdNode) (string, error) {
	switch n.Type {
	case "paragraph":
		return r.inline(n.Content)
	case "heading":
		s, err := r.inline(n.Content)
		if err != nil {
			return "", err
		}
		return strings.Repeat("#", min(max(n.num("level", 1), 1), 3)) + " " + s, nil
	case "bulletList":
		return r.list(n, func(int) string { return "- " })
	case "orderedList":
		start := n.num("start", 1)
		return r.list(n, func(i int) string { return strconv.Itoa(start+i) + ". " })
	case "taskList":
		return r.list(n, func(i int) string {
			if n.Content[i].Attrs["checked"] == true {
				return "- [x] "
			}
			return "- [ ] "
		})
	case "blockquote":
		inner, err := r.blocks(n.Content)
		if err != nil {
			return "", err
		}
		if inner == "" {
			return "", nil
		}
		return prefixLines(inner, "> ", "> "), nil
	case "callout":
		inner, err := r.blocks(n.Content)
		if err != nil {
			return "", err
		}
		return prefixLines(inner, "> "+n.str("emoji")+" ", "> "), nil
	case "horizontalRule":
		return "---", nil
	case "codeBlock":
		return codeBlock(n), nil
	case "table":
		return r.table(n)
	case "image":
		return "![" + escapeAlt(n.str("alt")) + "](" + r.l.Asset(n.str("assetId")) + ")", nil
	case "details":
		return r.details(n)
	}
	return "", fmt.Errorf("markdown: unknown block %q", n.Type)
}

// list renders the items of a list; marker gives the marker of item i.
func (r *mdRenderer) list(n *mdNode, marker func(i int) string) (string, error) {
	items := make([]string, 0, len(n.Content))
	for i, item := range n.Content {
		inner, err := r.itemBlocks(item.Content)
		if err != nil {
			return "", err
		}
		m := marker(i)
		indent := strings.Repeat(" ", len(m))
		if n.Type == "taskList" || n.Type == "bulletList" {
			indent = "  "
		}
		items = append(items, prefixLines(inner, m, indent))
	}
	return strings.Join(items, "\n"), nil
}

func codeBlock(n *mdNode) string {
	var code strings.Builder
	for _, c := range n.Content {
		code.WriteString(c.Text)
	}
	fence := strings.Repeat("`", max(3, longestRun(code.String(), '`')+1))
	s := fence + n.str("language") + "\n"
	if code.Len() > 0 {
		s += code.String() + "\n"
	}
	return s + fence
}

func longestRun(s string, c rune) int {
	best, cur := 0, 0
	for _, r := range s {
		if r == c {
			cur++
			best = max(best, cur)
		} else {
			cur = 0
		}
	}
	return best
}

func (r *mdRenderer) details(n *mdNode) (string, error) {
	summary, err := r.inline(n.Content[0].Content)
	if err != nil {
		return "", err
	}
	body, err := r.blocks(n.Content[1].Content)
	if err != nil {
		return "", err
	}
	s := "<details><summary>" + summary + "</summary>\n\n"
	if body != "" {
		s += body + "\n\n"
	}
	return s + "</details>", nil
}

// cell renders a table cell as one line.
func (r *mdRenderer) cell(n *mdNode) (string, error) {
	was := r.inTable
	r.inTable = true
	defer func() { r.inTable = was }()
	s, err := r.blocks(n.Content)
	if err != nil {
		return "", err
	}
	var lines []string
	for _, ln := range strings.Split(s, "\n") {
		if ln != "" {
			lines = append(lines, ln)
		}
	}
	return strings.Join(lines, "<br>"), nil
}

func (r *mdRenderer) table(n *mdNode) (string, error) {
	var rows [][]string
	cols := 0
	for _, row := range n.Content {
		cells := make([]string, len(row.Content))
		for i, c := range row.Content {
			s, err := r.cell(c)
			if err != nil {
				return "", err
			}
			cells[i] = s
		}
		cols = max(cols, len(cells))
		rows = append(rows, cells)
	}
	if len(rows) == 0 || cols == 0 {
		return "", nil
	}
	header := len(n.Content[0].Content) > 0 && n.Content[0].Content[0].Type == "tableHeader"
	var lines []string
	if header {
		lines = append(lines, tableRow(rows[0], cols))
		rows = rows[1:]
	} else {
		lines = append(lines, "|"+strings.Repeat(" |", cols))
	}
	lines = append(lines, "|"+strings.Repeat(" --- |", cols))
	for _, row := range rows {
		lines = append(lines, tableRow(row, cols))
	}
	return strings.Join(lines, "\n"), nil
}

func tableRow(cells []string, cols int) string {
	var b strings.Builder
	b.WriteString("|")
	for i := range cols {
		c := ""
		if i < len(cells) {
			c = cells[i]
		}
		b.WriteString(" " + c + " |")
	}
	return b.String()
}

// inline renders a run of inline nodes.
func (r *mdRenderer) inline(nodes []*mdNode) (string, error) {
	var b strings.Builder
	lineStart := true
	for _, n := range nodes {
		switch n.Type {
		case "text":
			b.WriteString(r.text(n, lineStart))
			lineStart = strings.HasSuffix(n.Text, "\n")
			continue
		case "hardBreak":
			if r.inTable {
				b.WriteString("<br>")
			} else {
				b.WriteString("\\\n")
			}
			lineStart = true
			continue
		case "mention":
			ref := doc.Ref{Kind: n.str("kind"), ID: n.str("id")}
			label := r.escape(r.l.Label(ref, n.str("label")), false)
			if ref.Kind == doc.KindPage {
				if href, ok := r.l.Page(ref.ID); ok {
					label = "[" + label + "](" + href + ")"
				}
			}
			b.WriteString(label)
		case "inlineRoll":
			b.WriteString("`[[" + n.str("formula") + "]]`")
		default:
			return "", fmt.Errorf("markdown: unknown inline node %q", n.Type)
		}
		lineStart = false
	}
	return b.String(), nil
}

// text renders a text node with its marks. Whitespace at the edges stays
// outside of the markers, which would not work otherwise.
func (r *mdRenderer) text(n *mdNode, lineStart bool) string {
	if len(n.Marks) == 0 {
		return r.escape(n.Text, lineStart)
	}
	core := strings.TrimFunc(n.Text, unicode.IsSpace)
	if core == "" {
		return r.escape(n.Text, lineStart)
	}
	lead := n.Text[:strings.Index(n.Text, core)]
	trail := n.Text[len(lead)+len(core):]

	has := map[string]mdMark{}
	for _, m := range n.Marks {
		has[m.Type] = m
	}
	var s string
	if _, ok := has["code"]; ok {
		s = codeSpan(core, r.inTable)
	} else {
		s = r.escape(core, lineStart && lead == "")
	}
	if _, ok := has["strike"]; ok {
		s = "~~" + s + "~~"
	}
	if _, ok := has["underline"]; ok {
		s = "<u>" + s + "</u>"
	}
	if _, ok := has["italic"]; ok {
		s = "*" + s + "*"
	}
	if _, ok := has["bold"]; ok {
		s = "**" + s + "**"
	}
	if m, ok := has["link"]; ok {
		href, _ := m.Attrs["href"].(string)
		s = "[" + s + "](" + escapeHref(href) + ")"
	}
	return r.escape(lead, lineStart) + s + r.escape(trail, false)
}

func codeSpan(s string, inTable bool) string {
	if inTable {
		s = strings.ReplaceAll(s, "|", `\|`)
	}
	fence := strings.Repeat("`", longestRun(s, '`')+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return fence + s + fence
}

// escape puts a backslash before the characters that would turn text into
// Markdown syntax; lineStart tells that s begins a line.
func (r *mdRenderer) escape(s string, lineStart bool) string {
	var b strings.Builder
	atStart := lineStart
	for i := 0; i < len(s); {
		c, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case strings.ContainsRune("\\*_`[]<>", c):
			b.WriteByte('\\')
		case c == '|' && r.inTable:
			b.WriteByte('\\')
		case atStart && c == '#':
			b.WriteByte('\\')
		case atStart && (c == '-' || c == '+') && isSpaceAt(s, i+size):
			b.WriteByte('\\')
		case atStart && unicode.IsDigit(c) && numberedMarker(s[i:]):
			// "1. " would start a list: escape the dot after the digits.
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			b.WriteString(s[i:j] + "\\")
			i = j
			atStart = false
			continue
		}
		b.WriteRune(c)
		atStart = c == '\n'
		i += size
	}
	return b.String()
}

func isSpaceAt(s string, i int) bool { return i < len(s) && (s[i] == ' ' || s[i] == '\t') }

// numberedMarker reports whether s starts with digits, a dot or bracket and a space.
func numberedMarker(s string) bool {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return i > 0 && i < len(s)-1 && (s[i] == '.' || s[i] == ')') && s[i+1] == ' '
}

func escapeAlt(s string) string {
	return strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "\n", " ").Replace(s)
}

func escapeHref(s string) string {
	return strings.NewReplacer(" ", "%20", "(", "%28", ")", "%29", "<", "%3C", ">", "%3E").Replace(s)
}
