package wiki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"
	"unicode/utf8"
)

const (
	maxFilters     = 20
	maxSorts       = 3
	minColumnWidth = 40
	maxColumnWidth = 2000
	maxFilterText  = 200
)

type filter struct {
	PropertyID string          `json:"propertyId,omitempty"`
	Field      string          `json:"field,omitempty"`
	Op         string          `json:"op"`
	Value      json.RawMessage `json:"value,omitempty"`
}

type sortKey struct {
	PropertyID string `json:"propertyId,omitempty"`
	Field      string `json:"field,omitempty"`
	Dir        string `json:"dir"`
}

// viewConfig is the stored form of the config of a view.
type viewConfig struct {
	Filters []filter       `json:"filters"`
	Sorts   []sortKey      `json:"sorts"`
	Visible []string       `json:"visible"`
	Widths  map[string]int `json:"widths"`
}

// Value kinds of the filter operators.
const (
	valNone   = "none"
	valText   = "text"
	valNumber = "number"
	valOption = "option"
	valDate   = "date"
	valTag    = "tag"
)

var (
	textOps   = map[string]string{"contains": valText, "empty": valNone, "not_empty": valNone}
	dateOps   = map[string]string{"before": valDate, "after": valDate, "eq": valDate, "empty": valNone}
	linkOps   = map[string]string{"empty": valNone, "not_empty": valNone}
	fieldDate = map[string]string{"before": valDate, "after": valDate, "eq": valDate}

	// propertyOps maps a property type to its filter operators.
	propertyOps = map[string]map[string]string{
		PropText: textOps, PropURL: textOps,
		PropNumber: {
			"eq": valNumber, "neq": valNumber, "lt": valNumber, "lte": valNumber, "gt": valNumber, "gte": valNumber,
			"empty": valNone,
		},
		PropSelect:      {"eq": valOption, "neq": valOption, "empty": valNone},
		PropMultiSelect: {"contains": valOption, "not_contains": valOption, "empty": valNone},
		PropCheckbox:    {"checked": valNone, "unchecked": valNone},
		PropDate:        dateOps,
		PropRelation:    linkOps, PropGameLink: linkOps,
	}

	// fieldOps maps a built-in field to its filter operators.
	fieldOps = map[string]map[string]string{
		"title":     textOps,
		"createdAt": fieldDate,
		"updatedAt": fieldDate,
		"tags":      {"contains": valTag},
	}

	sortFields = []string{"title", "createdAt", "updatedAt"}
)

func defaultConfig(props []Property) viewConfig {
	c := viewConfig{Filters: []filter{}, Sorts: []sortKey{}, Visible: make([]string, len(props)), Widths: map[string]int{}}
	for i, p := range props {
		c.Visible[i] = p.ID
	}
	return c
}

// normalizeConfig validates the config of a view of a database with props and
// returns its stored form; absent keys get their defaults (all properties visible).
func normalizeConfig(raw json.RawMessage, props []Property) (json.RawMessage, error) {
	cfg := defaultConfig(props)
	if !isNull(raw) {
		var in struct {
			Filters *[]filter       `json:"filters"`
			Sorts   *[]sortKey      `json:"sorts"`
			Visible *[]string       `json:"visible"`
			Widths  *map[string]int `json:"widths"`
		}
		if err := strictDecode(raw, &in, "config"); err != nil {
			return nil, err
		}
		byID := make(map[string]Property, len(props))
		for _, p := range props {
			byID[p.ID] = p
		}
		if in.Filters != nil && *in.Filters != nil {
			if len(*in.Filters) > maxFilters {
				return nil, invalid("config.filters: more than %d", maxFilters)
			}
			for _, f := range *in.Filters {
				nf, err := normalizeFilter(f, byID)
				if err != nil {
					return nil, err
				}
				cfg.Filters = append(cfg.Filters, nf)
			}
		}
		if in.Sorts != nil && *in.Sorts != nil {
			if len(*in.Sorts) > maxSorts {
				return nil, invalid("config.sorts: more than %d", maxSorts)
			}
			for _, s := range *in.Sorts {
				if err := checkSort(s, byID); err != nil {
					return nil, err
				}
				cfg.Sorts = append(cfg.Sorts, s)
			}
		}
		if in.Visible != nil && *in.Visible != nil {
			cfg.Visible = []string{}
			for _, pid := range *in.Visible {
				if _, ok := byID[pid]; !ok {
					return nil, invalid("config.visible: unknown property %q", pid)
				}
				if slices.Contains(cfg.Visible, pid) {
					return nil, invalid("config.visible: %q repeats", pid)
				}
				cfg.Visible = append(cfg.Visible, pid)
			}
		}
		if in.Widths != nil && *in.Widths != nil {
			for key, w := range *in.Widths {
				if _, ok := byID[key]; !ok && key != "title" {
					return nil, invalid("config.widths: unknown column %q", key)
				}
				if w < minColumnWidth || w > maxColumnWidth {
					return nil, invalid("config.widths: width must be %d to %d", minColumnWidth, maxColumnWidth)
				}
				cfg.Widths[key] = w
			}
		}
	}
	return marshal(cfg)
}

func checkSort(s sortKey, byID map[string]Property) error {
	if (s.PropertyID == "") == (s.Field == "") {
		return invalid("config.sorts: exactly one of propertyId and field is required")
	}
	if s.PropertyID != "" {
		if _, ok := byID[s.PropertyID]; !ok {
			return invalid("config.sorts: unknown property %q", s.PropertyID)
		}
	} else if !slices.Contains(sortFields, s.Field) {
		return invalid("config.sorts: unknown field %q", s.Field)
	}
	if s.Dir != "asc" && s.Dir != "desc" {
		return invalid("config.sorts: dir must be asc or desc")
	}
	return nil
}

func normalizeFilter(f filter, byID map[string]Property) (filter, error) {
	if (f.PropertyID == "") == (f.Field == "") {
		return filter{}, invalid("config.filters: exactly one of propertyId and field is required")
	}
	var (
		ops  map[string]string
		prop Property
	)
	if f.PropertyID != "" {
		p, ok := byID[f.PropertyID]
		if !ok {
			return filter{}, invalid("config.filters: unknown property %q", f.PropertyID)
		}
		prop, ops = p, propertyOps[p.Type]
	} else {
		var ok bool
		if ops, ok = fieldOps[f.Field]; !ok {
			return filter{}, invalid("config.filters: unknown field %q", f.Field)
		}
	}
	kind, ok := ops[f.Op]
	if !ok {
		return filter{}, invalid("config.filters: operator %q is not allowed here", f.Op)
	}
	out := filter{PropertyID: f.PropertyID, Field: f.Field, Op: f.Op}
	if kind == valNone {
		if !isNull(f.Value) {
			return filter{}, invalid("config.filters: operator %q takes no value", f.Op)
		}
		return out, nil
	}
	if isNull(f.Value) {
		return filter{}, invalid("config.filters: operator %q needs a value", f.Op)
	}
	v, err := decodeNumbers(f.Value, "config.filters value")
	if err != nil {
		return filter{}, err
	}
	var norm any
	switch kind {
	case valText:
		s, ok := v.(string)
		if !ok || !utf8.ValidString(s) || utf8.RuneCountInString(s) < 1 || utf8.RuneCountInString(s) > maxFilterText {
			return filter{}, invalid("config.filters: value must be a string of 1 to %d characters", maxFilterText)
		}
		norm = s
	case valNumber:
		n, _, ok := finiteNumber(v)
		if !ok {
			return filter{}, invalid("config.filters: value must be a finite number")
		}
		norm = n
	case valOption:
		s, ok := v.(string)
		known, err := optionIDs(prop.Options)
		if err != nil {
			return filter{}, err
		}
		if !ok || !known[s] {
			return filter{}, invalid("config.filters: value must be a variant of the property")
		}
		norm = s
	case valDate:
		s, ok := v.(string)
		if !ok {
			return filter{}, invalid("config.filters: value must be a YYYY-MM-DD date")
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return filter{}, invalid("config.filters: value must be a YYYY-MM-DD date")
		}
		norm = s
	case valTag:
		s, ok := v.(string)
		if !ok || s == "" {
			return filter{}, invalid("config.filters: value must be a tag id")
		}
		norm = s
	}
	if out.Value, err = marshal(norm); err != nil {
		return filter{}, err
	}
	return out, nil
}

// dropProperty removes everything that refers to the property.
func (c *viewConfig) dropProperty(propID string) {
	c.Filters = slices.DeleteFunc(c.Filters, func(f filter) bool { return f.PropertyID == propID })
	c.Sorts = slices.DeleteFunc(c.Sorts, func(s sortKey) bool { return s.PropertyID == propID })
	c.Visible = slices.DeleteFunc(c.Visible, func(v string) bool { return v == propID })
	delete(c.Widths, propID)
}

// dropVariants removes the filters of the property that compare with a removed variant.
func (c *viewConfig) dropVariants(propID string, removed map[string]bool) {
	c.Filters = slices.DeleteFunc(c.Filters, func(f filter) bool {
		if f.PropertyID != propID {
			return false
		}
		var s string
		return json.Unmarshal(f.Value, &s) == nil && removed[s]
	})
}

// rewriteViews lets fn edit the config of every view of the database and saves the changed ones.
func rewriteViews(ctx context.Context, q querier, dbID string, fn func(c *viewConfig)) error {
	views, err := listViews(ctx, q, dbID)
	if err != nil {
		return err
	}
	for _, v := range views {
		var c viewConfig
		if err := json.Unmarshal(v.Config, &c); err != nil {
			return fmt.Errorf("parse view config: %w", err)
		}
		fn(&c)
		if c.Filters == nil {
			c.Filters = []filter{}
		}
		if c.Sorts == nil {
			c.Sorts = []sortKey{}
		}
		if c.Visible == nil {
			c.Visible = []string{}
		}
		if c.Widths == nil {
			c.Widths = map[string]int{}
		}
		out, err := json.Marshal(c)
		if err != nil {
			return fmt.Errorf("marshal view config: %w", err)
		}
		if bytes.Equal(out, v.Config) {
			continue
		}
		if _, err := q.ExecContext(ctx, `UPDATE wiki_view SET config = ? WHERE id = ?`, string(out), v.ID); err != nil {
			return fmt.Errorf("update view config: %w", err)
		}
	}
	return nil
}
