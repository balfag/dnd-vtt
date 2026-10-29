package doc

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/dice"
)

var (
	languageRe = regexp.MustCompile(`^[a-z0-9+#-]{1,20}$`)
	ulidRe     = regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`)
)

const (
	maxLabelRunes   = 200
	maxAltRunes     = 200
	maxFormulaRunes = 200
	maxHrefRunes    = 2000
)

// attrSpec describes one allowed attribute. check validates and normalizes a
// non-null value; a null or missing value yields def, or an error if required.
type attrSpec struct {
	name     string
	required bool
	def      any
	check    func(v any) (any, error)
}

// contentRule describes the children of a node.
type contentRule struct {
	kinds   map[string]bool // allowed child types
	min     int             // minimum number of children
	first   string          // if set, the first child must be of this type
	exact   []string        // if set, the children must be exactly these types
	noMarks bool            // text children must carry no marks
}

// nodeSpec describes one node type. A nil content means the node has no children.
type nodeSpec struct {
	content *contentRule
	attrs   []attrSpec
}

func set(types ...string) map[string]bool {
	m := make(map[string]bool, len(types))
	for _, t := range types {
		m[t] = true
	}
	return m
}

var (
	blockTypes  = set("paragraph", "heading", "bulletList", "orderedList", "taskList", "blockquote", "callout", "horizontalRule", "codeBlock", "table", "image", "details")
	inlineTypes = set("text", "hardBreak", "mention", "inlineRoll")
)

func intAttr(name string, lo, hi int64, required bool, def any) attrSpec {
	return attrSpec{name: name, required: required, def: def, check: func(v any) (any, error) {
		n, err := toInt(v)
		if err != nil {
			return nil, err
		}
		if n < lo || n > hi {
			return nil, fmt.Errorf("must be between %d and %d", lo, hi)
		}
		return n, nil
	}}
}

func boolAttr(name string) attrSpec {
	return attrSpec{name: name, def: false, check: func(v any) (any, error) {
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("must be a boolean")
		}
		return b, nil
	}}
}

func stringAttr(name string, required bool, def string, check func(s string) error) attrSpec {
	return attrSpec{name: name, required: required, def: def, check: func(v any) (any, error) {
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("must be a string")
		}
		if err := check(s); err != nil {
			return nil, err
		}
		return s, nil
	}}
}

func runesAtMost(limit int) func(string) error {
	return func(s string) error {
		if utf8.RuneCountInString(s) > limit {
			return fmt.Errorf("must be at most %d characters", limit)
		}
		return nil
	}
}

// toInt converts a JSON number without a fractional part to int64.
func toInt(v any) (int64, error) {
	num, ok := v.(interface {
		Int64() (int64, error)
		Float64() (float64, error)
	})
	if !ok {
		return 0, fmt.Errorf("must be an integer")
	}
	if n, err := num.Int64(); err == nil {
		return n, nil
	}
	f, err := num.Float64()
	if err != nil || f != math.Trunc(f) || math.Abs(f) > 1<<53 {
		return 0, fmt.Errorf("must be an integer")
	}
	return int64(f), nil
}

var colwidthAttr = attrSpec{name: "colwidth", def: nil, check: func(v any) (any, error) {
	arr, ok := v.([]any)
	if !ok || len(arr) != 1 {
		return nil, fmt.Errorf("must be null or an array of one integer")
	}
	n, err := toInt(arr[0])
	if err != nil {
		return nil, err
	}
	if n < 40 || n > 2000 {
		return nil, fmt.Errorf("width must be between 40 and 2000")
	}
	return []int64{n}, nil
}}

var cellAttrs = []attrSpec{
	intAttr("colspan", 1, 1, false, int64(1)),
	intAttr("rowspan", 1, 1, false, int64(1)),
	colwidthAttr,
}

var nodeSpecs = map[string]nodeSpec{
	"doc":            {content: &contentRule{kinds: blockTypes, min: 1}},
	"paragraph":      {content: &contentRule{kinds: inlineTypes}},
	"heading":        {content: &contentRule{kinds: inlineTypes}, attrs: []attrSpec{intAttr("level", 1, 3, true, nil)}},
	"bulletList":     {content: &contentRule{kinds: set("listItem"), min: 1}},
	"orderedList":    {content: &contentRule{kinds: set("listItem"), min: 1}, attrs: []attrSpec{intAttr("start", 1, 10000, false, int64(1))}},
	"listItem":       {content: &contentRule{kinds: blockTypes, min: 1, first: "paragraph"}},
	"taskList":       {content: &contentRule{kinds: set("taskItem"), min: 1}},
	"taskItem":       {content: &contentRule{kinds: blockTypes, min: 1, first: "paragraph"}, attrs: []attrSpec{boolAttr("checked")}},
	"blockquote":     {content: &contentRule{kinds: blockTypes, min: 1}},
	"callout":        {content: &contentRule{kinds: blockTypes, min: 1}, attrs: []attrSpec{stringAttr("emoji", false, "💡", checkEmoji)}},
	"horizontalRule": {},
	"codeBlock":      {content: &contentRule{kinds: set("text"), noMarks: true}, attrs: []attrSpec{stringAttr("language", false, "", checkLanguage)}},
	"table":          {content: &contentRule{kinds: set("tableRow"), min: 1}},
	"tableRow":       {content: &contentRule{kinds: set("tableCell", "tableHeader"), min: 1}},
	"tableCell":      {content: &contentRule{kinds: blockTypes, min: 1}, attrs: cellAttrs},
	"tableHeader":    {content: &contentRule{kinds: blockTypes, min: 1}, attrs: cellAttrs},
	"image": {attrs: []attrSpec{
		stringAttr("assetId", true, "", nonEmpty),
		stringAttr("alt", false, "", runesAtMost(maxAltRunes)),
	}},
	"details":        {content: &contentRule{exact: []string{"detailsSummary", "detailsContent"}}, attrs: []attrSpec{boolAttr("open")}},
	"detailsSummary": {content: &contentRule{kinds: inlineTypes}},
	"detailsContent": {content: &contentRule{kinds: blockTypes, min: 1}},
	"text":           {},
	"hardBreak":      {},
	"mention": {attrs: []attrSpec{
		stringAttr("kind", true, "", checkKind),
		stringAttr("id", true, "", checkULID),
		stringAttr("label", false, "", runesAtMost(maxLabelRunes)),
	}},
	"inlineRoll": {attrs: []attrSpec{stringAttr("formula", true, "", checkFormula)}},
}

var markSpecs = map[string][]attrSpec{
	"bold":      nil,
	"italic":    nil,
	"underline": nil,
	"strike":    nil,
	"code":      nil,
	"link":      {stringAttr("href", true, "", checkHref)},
}

func nonEmpty(s string) error {
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	return nil
}

func checkEmoji(s string) error {
	if s == "" || !ValidIcon(s) {
		return fmt.Errorf("must be 1 to 8 characters without control characters")
	}
	return nil
}

func checkLanguage(s string) error {
	if s != "" && !languageRe.MatchString(s) {
		return fmt.Errorf("must be empty or 1 to 20 characters of [a-z0-9+#-]")
	}
	return nil
}

func checkKind(s string) error {
	switch s {
	case KindPage, KindScene, KindToken:
		return nil
	}
	return fmt.Errorf("must be page, scene or token")
}

func checkULID(s string) error {
	if !ulidRe.MatchString(s) {
		return fmt.Errorf("must be a ULID")
	}
	return nil
}

func checkFormula(s string) error {
	n := utf8.RuneCountInString(s)
	if n < 1 || n > maxFormulaRunes {
		return fmt.Errorf("must be 1 to %d characters", maxFormulaRunes)
	}
	if _, err := dice.Parse(s); err != nil {
		return fmt.Errorf("invalid dice formula: %w", err)
	}
	return nil
}

func checkHref(s string) error {
	if s == "" || utf8.RuneCountInString(s) > maxHrefRunes {
		return fmt.Errorf("must be 1 to %d characters", maxHrefRunes)
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https", "mailto":
		return nil
	}
	return fmt.Errorf("scheme must be http, https or mailto")
}
