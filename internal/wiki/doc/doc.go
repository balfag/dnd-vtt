// Package doc validates and normalizes wiki documents (ProseMirror JSON, wiki schema v1).
package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unicode"
	"unicode/utf8"
)

// Target kinds of a mention.
const (
	KindPage  = "page"
	KindScene = "scene"
	KindToken = "token"
)

const (
	MaxPageBytes     = 1 << 20   // raw JSON of a page or template
	MaxHandoutBytes  = 256 << 10 // raw JSON of a handout
	MaxHandoutImages = 20
	MaxDepth         = 50    // the doc node is depth 1
	MaxNodes         = 50000 // every node object counts, text nodes included
)

// ErrInvalid is wrapped by every validation error: fmt.Errorf("%w: <path>: <reason>", ErrInvalid).
// <path> is like "content[2].content[0].attrs.level".
var ErrInvalid = errors.New("doc: invalid document")

// Ref is a mention target.
type Ref struct{ Kind, ID string }

type Options struct {
	// Handout forbids mention and inlineRoll, allows at most MaxHandoutImages
	// image nodes and MaxHandoutBytes of raw JSON (instead of MaxPageBytes).
	Handout bool
	// AssetOK reports whether id is an asset of the document's campaign.
	// Required (nil panics). Its error is returned as is (not ErrInvalid).
	// It is called once per distinct assetId.
	AssetOK func(id string) (bool, error)
}

type Result struct {
	JSON     []byte   // normalized compact JSON
	Text     string   // plain text for search
	Mentions []Ref    // distinct, in document order
	Assets   []string // distinct image assetIds, in document order
}

// node and mark are the canonical form of a document; their field order is the
// key order of the normalized JSON.
type node struct {
	Type    string         `json:"type"`
	Attrs   map[string]any `json:"attrs,omitempty"`
	Content []*node        `json:"content,omitempty"`
	Marks   []mark         `json:"marks,omitempty"`
	Text    string         `json:"text,omitempty"`
}

type mark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs,omitempty"`
}

var emptyDoc = []byte(`{"type":"doc","content":[{"type":"paragraph"}]}`)

// Empty returns the empty document in normalized form (a new slice each call).
func Empty() []byte { return bytes.Clone(emptyDoc) }

// ValidIcon reports whether s is a valid page or callout icon: valid UTF-8,
// 0..8 code points, no control characters.
func ValidIcon(s string) bool {
	if !utf8.ValidString(s) || utf8.RuneCountInString(s) > 8 {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

type assetRef struct{ id, path string }

type walker struct {
	opts     Options
	nodes    int
	images   int
	mentions []Ref
	seenMen  map[Ref]bool
	assets   []assetRef
	seenAss  map[string]bool
}

func invalid(path, format string, args ...any) error {
	if path == "" {
		path = "<root>"
	}
	return fmt.Errorf("%w: %s: %s", ErrInvalid, path, fmt.Sprintf(format, args...))
}

func join(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

// Validate parses raw, checks it against the wiki schema v1 and normalizes it.
func Validate(raw []byte, opts Options) (Result, error) {
	if opts.AssetOK == nil {
		panic("doc: Options.AssetOK is required")
	}
	limit := MaxPageBytes
	if opts.Handout {
		limit = MaxHandoutBytes
	}
	if len(raw) > limit {
		return Result{}, invalid("", "document is larger than %d bytes", limit)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return Result{}, invalid("", "malformed JSON: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return Result{}, invalid("", "unexpected data after the document")
	}

	w := &walker{opts: opts, seenMen: map[Ref]bool{}, seenAss: map[string]bool{}}
	root, err := w.node(v, "", 1, map[string]bool{"doc": true})
	if err != nil {
		return Result{}, err
	}

	for _, a := range w.assets {
		ok, err := opts.AssetOK(a.id)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			return Result{}, invalid(a.path, "asset %q does not belong to the campaign", a.id)
		}
	}

	out, err := json.Marshal(root)
	if err != nil {
		return Result{}, fmt.Errorf("doc: marshal: %w", err)
	}
	res := Result{JSON: out, Text: extractText(root), Mentions: w.mentions, Assets: make([]string, len(w.assets))}
	if res.Mentions == nil {
		res.Mentions = []Ref{}
	}
	for i, a := range w.assets {
		res.Assets[i] = a.id
	}
	return res, nil
}

// node validates and normalizes one node. allowed is the set of types the
// parent permits at this position.
func (w *walker) node(v any, path string, depth int, allowed map[string]bool) (*node, error) {
	if depth > MaxDepth {
		return nil, invalid(path, "nesting is deeper than %d", MaxDepth)
	}
	w.nodes++
	if w.nodes > MaxNodes {
		return nil, invalid(path, "more than %d nodes", MaxNodes)
	}
	obj, ok := v.(map[string]any)
	if !ok {
		return nil, invalid(path, "node must be an object")
	}
	typ, ok := obj["type"].(string)
	if !ok {
		return nil, invalid(join(path, "type"), "must be a string")
	}
	spec, known := nodeSpecs[typ]
	if !known {
		return nil, invalid(join(path, "type"), "unknown node type %q", typ)
	}
	if !allowed[typ] {
		return nil, invalid(join(path, "type"), "node %q is not allowed here", typ)
	}

	n := &node{Type: typ}
	attrs, err := parseAttrs(spec.attrs, obj["attrs"], join(path, "attrs"))
	if err != nil {
		return nil, err
	}
	n.Attrs = attrs
	if err := w.track(n, path); err != nil {
		return nil, err
	}

	if typ == "text" {
		s, ok := obj["text"].(string)
		if !ok || s == "" {
			return nil, invalid(join(path, "text"), "must be a non-empty string")
		}
		n.Text = s
		marks, err := parseMarks(obj["marks"], join(path, "marks"))
		if err != nil {
			return nil, err
		}
		n.Marks = marks
	}

	children, err := optionalArray(obj["content"], join(path, "content"))
	if err != nil {
		return nil, err
	}
	if spec.content == nil {
		if len(children) > 0 {
			return nil, invalid(join(path, "content"), "node %q cannot have content", typ)
		}
		return n, nil
	}
	rule := spec.content
	for i, c := range children {
		cpath := fmt.Sprintf("%s[%d]", join(path, "content"), i)
		child, err := w.node(c, cpath, depth+1, rule.allowedAt(i))
		if err != nil {
			return nil, err
		}
		if rule.noMarks && len(child.Marks) > 0 {
			return nil, invalid(join(cpath, "marks"), "marks are not allowed in %q", typ)
		}
		n.Content = append(n.Content, child)
	}
	min := rule.min
	if rule.exact != nil {
		min = len(rule.exact)
		if len(children) > min {
			return nil, invalid(join(path, "content"), "node %q must have exactly %d children", typ, min)
		}
	}
	if len(children) < min {
		return nil, invalid(join(path, "content"), "node %q needs at least %d children", typ, min)
	}
	return n, nil
}

func (r *contentRule) allowedAt(i int) map[string]bool {
	switch {
	case r.exact != nil:
		if i < len(r.exact) {
			return map[string]bool{r.exact[i]: true}
		}
		return nil
	case r.first != "" && i == 0:
		return map[string]bool{r.first: true}
	}
	return r.kinds
}

// track records mentions and images and applies the handout restrictions.
func (w *walker) track(n *node, path string) error {
	switch n.Type {
	case "mention", "inlineRoll":
		if w.opts.Handout {
			return invalid(join(path, "type"), "%q is not allowed in a handout", n.Type)
		}
		if n.Type == "mention" {
			ref := Ref{Kind: n.Attrs["kind"].(string), ID: n.Attrs["id"].(string)}
			if !w.seenMen[ref] {
				w.seenMen[ref] = true
				w.mentions = append(w.mentions, ref)
			}
		}
	case "image":
		w.images++
		if w.opts.Handout && w.images > MaxHandoutImages {
			return invalid(join(path, "type"), "a handout can have at most %d images", MaxHandoutImages)
		}
		id := n.Attrs["assetId"].(string)
		if !w.seenAss[id] {
			w.seenAss[id] = true
			w.assets = append(w.assets, assetRef{id: id, path: join(path, "attrs.assetId")})
		}
	}
	return nil
}

func optionalArray(v any, path string) ([]any, error) {
	if v == nil {
		return nil, nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil, invalid(path, "must be an array")
	}
	return arr, nil
}

// parseAttrs keeps only the allowed attributes, fills in defaults and validates values.
func parseAttrs(specs []attrSpec, raw any, path string) (map[string]any, error) {
	var in map[string]any
	if raw != nil {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, invalid(path, "must be an object")
		}
		in = m
	}
	if len(specs) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(specs))
	for _, s := range specs {
		v := in[s.name]
		if v == nil {
			if s.required {
				return nil, invalid(join(path, s.name), "is required")
			}
			out[s.name] = s.def
			continue
		}
		nv, err := s.check(v)
		if err != nil {
			return nil, invalid(join(path, s.name), "%v", err)
		}
		out[s.name] = nv
	}
	return out, nil
}

func parseMarks(raw any, path string) ([]mark, error) {
	arr, err := optionalArray(raw, path)
	if err != nil {
		return nil, err
	}
	var marks []mark
	for i, m := range arr {
		mpath := path + "[" + strconv.Itoa(i) + "]"
		obj, ok := m.(map[string]any)
		if !ok {
			return nil, invalid(mpath, "mark must be an object")
		}
		typ, ok := obj["type"].(string)
		if !ok {
			return nil, invalid(join(mpath, "type"), "must be a string")
		}
		specs, known := markSpecs[typ]
		if !known {
			return nil, invalid(join(mpath, "type"), "unknown mark type %q", typ)
		}
		attrs, err := parseAttrs(specs, obj["attrs"], join(mpath, "attrs"))
		if err != nil {
			return nil, err
		}
		marks = append(marks, mark{Type: typ, Attrs: attrs})
	}
	return marks, nil
}
