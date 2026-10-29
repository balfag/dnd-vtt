package wiki

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"math"
	"net/url"
	"slices"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/balfag/dnd-vtt/internal/id"
)

const (
	maxTextValue     = 2000 // code points
	maxURLValue      = 2000
	maxMultiValues   = 20
	maxRelationLinks = 50
	maxGameLinks     = 20
	maxOptionName    = 100
)

var propTypes = []string{
	PropText, PropNumber, PropSelect, PropMultiSelect, PropCheckbox, PropDate, PropURL, PropRelation, PropGameLink,
}

var optionColors = []string{"gray", "brown", "orange", "yellow", "green", "blue", "purple", "red"}

type optionJSON struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type selectOptions struct {
	Options []optionJSON `json:"options"`
}

type relationOptions struct {
	TargetDatabaseID string `json:"targetDatabaseId"`
}

func isNull(raw []byte) bool {
	t := bytes.TrimSpace(raw)
	return len(t) == 0 || bytes.Equal(t, []byte("null"))
}

func marshal(v any) (json.RawMessage, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal json: %w", err)
	}
	return b, nil
}

// strictDecode decodes the single JSON value raw into dst and rejects unknown keys.
func strictDecode(raw []byte, dst any, what string) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return invalid("%s: %v", what, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return invalid("%s: unexpected data after the JSON value", what)
	}
	return nil
}

// decodeNumbers decodes a JSON value keeping numbers as json.Number.
func decodeNumbers(raw []byte, what string) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, invalid("%s: %v", what, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, invalid("%s: unexpected data after the JSON value", what)
	}
	return v, nil
}

func finiteNumber(v any) (json.Number, float64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return "", 0, false
	}
	f, err := strconv.ParseFloat(string(n), 64)
	if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
		return "", 0, false
	}
	return n, f, true
}

// optionIDs returns the variant ids of stored select options (nil for other types).
func optionIDs(raw json.RawMessage) (map[string]bool, error) {
	var so selectOptions
	if err := json.Unmarshal(raw, &so); err != nil {
		return nil, fmt.Errorf("parse property options: %w", err)
	}
	out := make(map[string]bool, len(so.Options))
	for _, o := range so.Options {
		out[o.ID] = true
	}
	return out, nil
}

// normalizeOptions validates the options of a property of the given type and
// returns their stored form. prev holds the variant ids that already exist.
func normalizeOptions(ctx context.Context, q querier, campaignID, typ string, raw json.RawMessage, prev map[string]bool) (json.RawMessage, error) {
	absent := isNull(raw)
	switch typ {
	case PropNumber:
		var in struct {
			Format *string `json:"format"`
		}
		if !absent {
			if err := strictDecode(raw, &in, "options"); err != nil {
				return nil, err
			}
		}
		format := "decimal"
		if in.Format != nil {
			if *in.Format != "integer" && *in.Format != "decimal" {
				return nil, invalid("options.format: must be integer or decimal")
			}
			format = *in.Format
		}
		return marshal(map[string]string{"format": format})

	case PropSelect, PropMultiSelect:
		var in struct {
			Options *[]optionJSON `json:"options"`
		}
		if !absent {
			if err := strictDecode(raw, &in, "options"); err != nil {
				return nil, err
			}
		}
		list := []optionJSON{}
		if in.Options != nil && *in.Options != nil {
			list = *in.Options
		}
		if len(list) > MaxOptions {
			return nil, invalid("options: more than %d variants", MaxOptions)
		}
		seen := map[string]bool{}
		for i := range list {
			o := &list[i]
			if !utf8.ValidString(o.Name) {
				return nil, invalid("options: variant name is not valid UTF-8")
			}
			if n := utf8.RuneCountInString(o.Name); n < 1 || n > maxOptionName {
				return nil, invalid("options: variant name must be 1 to %d characters", maxOptionName)
			}
			if !slices.Contains(optionColors, o.Color) {
				return nil, invalid("options: unknown color %q", o.Color)
			}
			if o.ID == "" {
				o.ID = id.New()
			} else if !prev[o.ID] {
				return nil, invalid("options: unknown variant id %q", o.ID)
			}
			if seen[o.ID] {
				return nil, invalid("options: variant id %q repeats", o.ID)
			}
			seen[o.ID] = true
		}
		return marshal(selectOptions{Options: list})

	case PropRelation:
		if absent {
			return nil, invalid("options.targetDatabaseId: required")
		}
		var in relationOptions
		if err := strictDecode(raw, &in, "options"); err != nil {
			return nil, err
		}
		if in.TargetDatabaseID == "" {
			return nil, invalid("options.targetDatabaseId: required")
		}
		var one int
		err := q.QueryRowContext(ctx, `
			SELECT 1 FROM wiki_page WHERE id = ? AND campaign_id = ? AND kind = 'database' AND deleted_at IS NULL`,
			in.TargetDatabaseID, campaignID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, invalid("options.targetDatabaseId: no such database")
		}
		if err != nil {
			return nil, fmt.Errorf("find target database: %w", err)
		}
		return marshal(in)

	default:
		if !absent {
			var in struct{}
			if err := strictDecode(raw, &in, "options"); err != nil {
				return nil, err
			}
		}
		return json.RawMessage(`{}`), nil
	}
}

// applyOptionChange cleans the data that depends on the old options of p after
// they became newOpts: removed variants disappear from records and view
// filters; a new relation target drops the values of the property.
func (s *Service) applyOptionChange(ctx context.Context, q querier, dbID string, p Property, newOpts json.RawMessage, prev map[string]bool) error {
	switch p.Type {
	case PropSelect, PropMultiSelect:
		kept, err := optionIDs(newOpts)
		if err != nil {
			return err
		}
		removed := map[string]bool{}
		for oid := range prev {
			if !kept[oid] {
				removed[oid] = true
			}
		}
		if len(removed) == 0 {
			return nil
		}
		if err := rewriteRecordProps(ctx, q, dbID, func(props map[string]any) bool {
			return dropVariants(props, p.ID, p.Type, removed)
		}); err != nil {
			return err
		}
		return rewriteViews(ctx, q, dbID, func(c *viewConfig) { c.dropVariants(p.ID, removed) })

	case PropRelation:
		var oldT, newT relationOptions
		if err := json.Unmarshal(p.Options, &oldT); err != nil {
			return fmt.Errorf("parse property options: %w", err)
		}
		if err := json.Unmarshal(newOpts, &newT); err != nil {
			return fmt.Errorf("parse property options: %w", err)
		}
		if oldT.TargetDatabaseID == newT.TargetDatabaseID {
			return nil
		}
		return rewriteRecordProps(ctx, q, dbID, func(props map[string]any) bool {
			_, had := props[p.ID]
			delete(props, p.ID)
			return had
		})
	}
	return nil
}

// dropVariants removes the variants from the value of property propID.
func dropVariants(props map[string]any, propID, typ string, removed map[string]bool) bool {
	v, ok := props[propID]
	if !ok {
		return false
	}
	if typ == PropSelect {
		if s, isStr := v.(string); isStr && removed[s] {
			delete(props, propID)
			return true
		}
		return false
	}
	list, _ := v.([]any)
	kept := make([]any, 0, len(list))
	for _, x := range list {
		if s, isStr := x.(string); isStr && removed[s] {
			continue
		}
		kept = append(kept, x)
	}
	if len(kept) == len(list) {
		return false
	}
	if len(kept) == 0 {
		delete(props, propID)
	} else {
		props[propID] = kept
	}
	return true
}

// rewriteRecordProps lets fn edit the props of every record of the database
// (the ones in the trash included); changed records are saved and their
// property links are recomputed.
func rewriteRecordProps(ctx context.Context, q querier, dbID string, fn func(props map[string]any) bool) error {
	rows, err := q.QueryContext(ctx, `SELECT id, props FROM wiki_page WHERE parent_id = ?`, dbID)
	if err != nil {
		return fmt.Errorf("list records: %w", err)
	}
	type rec struct{ id, props string }
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.props); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan record: %w", err)
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("list records: %w", err)
	}
	_ = rows.Close()

	types, err := propertyTypes(ctx, q, dbID)
	if err != nil {
		return err
	}
	for _, r := range recs {
		props, err := decodeProps(r.props)
		if err != nil {
			return err
		}
		if !fn(props) {
			continue
		}
		out, err := json.Marshal(props)
		if err != nil {
			return fmt.Errorf("marshal props: %w", err)
		}
		if _, err := q.ExecContext(ctx, `UPDATE wiki_page SET props = ? WHERE id = ?`, string(out), r.id); err != nil {
			return fmt.Errorf("update props: %w", err)
		}
		if err := rebuildLinks(ctx, q, r.id, out, types); err != nil {
			return err
		}
	}
	return nil
}

func decodeProps(raw string) (map[string]any, error) {
	v, err := decodeNumbers([]byte(raw), "props")
	if err != nil {
		return nil, fmt.Errorf("stored props: %w", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("stored props: not an object")
	}
	return m, nil
}

func propertyTypes(ctx context.Context, q querier, dbID string) (map[string]string, error) {
	props, err := listProperties(ctx, q, dbID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(props))
	for _, p := range props {
		out[p.ID] = p.Type
	}
	return out, nil
}

// rebuildLinks rewrites the 'property' links of a record from its props.
func rebuildLinks(ctx context.Context, q querier, recordID string, props []byte, types map[string]string) error {
	if _, err := q.ExecContext(ctx, `DELETE FROM wiki_link WHERE page_id = ? AND source = 'property'`, recordID); err != nil {
		return fmt.Errorf("clear property links: %w", err)
	}
	m, err := decodeProps(string(props))
	if err != nil {
		return err
	}
	insert := func(kind, target string) error {
		if _, err := q.ExecContext(ctx,
			`INSERT OR IGNORE INTO wiki_link (page_id, target_kind, target_id, source) VALUES (?, ?, ?, 'property')`,
			recordID, kind, target); err != nil {
			return fmt.Errorf("insert property link: %w", err)
		}
		return nil
	}
	for key, v := range m {
		list, _ := v.([]any)
		switch types[key] {
		case PropRelation:
			for _, x := range list {
				if target, ok := x.(string); ok {
					if err := insert(KindPage, target); err != nil {
						return err
					}
				}
			}
		case PropGameLink:
			for _, x := range list {
				if o, ok := x.(map[string]any); ok {
					kind, _ := o["kind"].(string)
					target, _ := o["id"].(string)
					if err := insert(kind, target); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

type gameLink struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

func stringList(v any, what string, minLen, maxLen int) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, invalid("%s: must be an array", what)
	}
	if len(list) < minLen || len(list) > maxLen {
		return nil, invalid("%s: must have %d to %d items", what, minLen, maxLen)
	}
	out := make([]string, len(list))
	seen := map[string]bool{}
	for i, x := range list {
		s, ok := x.(string)
		if !ok || s == "" {
			return nil, invalid("%s: items must be non-empty strings", what)
		}
		if seen[s] {
			return nil, invalid("%s: %q repeats", what, s)
		}
		seen[s] = true
		out[i] = s
	}
	return out, nil
}

// checkValue validates one value of property p and returns its stored form.
func checkValue(ctx context.Context, q querier, campaignID string, p Property, raw json.RawMessage) (any, error) {
	what := "props." + p.Name
	v, err := decodeNumbers(raw, what)
	if err != nil {
		return nil, err
	}
	switch p.Type {
	case PropText:
		s, ok := v.(string)
		if !ok || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxTextValue {
			return nil, invalid("%s: must be a string of up to %d characters", what, maxTextValue)
		}
		return s, nil

	case PropNumber:
		n, f, ok := finiteNumber(v)
		if !ok {
			return nil, invalid("%s: must be a finite number", what)
		}
		var opts struct {
			Format string `json:"format"`
		}
		if err := json.Unmarshal(p.Options, &opts); err != nil {
			return nil, fmt.Errorf("parse property options: %w", err)
		}
		if opts.Format == "integer" && f != math.Trunc(f) {
			return nil, invalid("%s: must be an integer", what)
		}
		return n, nil

	case PropSelect:
		s, ok := v.(string)
		if !ok {
			return nil, invalid("%s: must be a variant id", what)
		}
		known, err := optionIDs(p.Options)
		if err != nil {
			return nil, err
		}
		if !known[s] {
			return nil, invalid("%s: unknown variant", what)
		}
		return s, nil

	case PropMultiSelect:
		ids, err := stringList(v, what, 1, maxMultiValues)
		if err != nil {
			return nil, err
		}
		known, err := optionIDs(p.Options)
		if err != nil {
			return nil, err
		}
		for _, s := range ids {
			if !known[s] {
				return nil, invalid("%s: unknown variant", what)
			}
		}
		return ids, nil

	case PropCheckbox:
		b, ok := v.(bool)
		if !ok {
			return nil, invalid("%s: must be true or false", what)
		}
		return b, nil

	case PropDate:
		s, ok := v.(string)
		if !ok {
			return nil, invalid("%s: must be a YYYY-MM-DD date", what)
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, invalid("%s: must be a YYYY-MM-DD date", what)
		}
		return s, nil

	case PropURL:
		s, ok := v.(string)
		if !ok || !utf8.ValidString(s) || utf8.RuneCountInString(s) > maxURLValue {
			return nil, invalid("%s: must be a URL of up to %d characters", what, maxURLValue)
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, invalid("%s: must be an http or https URL", what)
		}
		return s, nil

	case PropRelation:
		ids, err := stringList(v, what, 1, maxRelationLinks)
		if err != nil {
			return nil, err
		}
		var target relationOptions
		if err := json.Unmarshal(p.Options, &target); err != nil {
			return nil, fmt.Errorf("parse property options: %w", err)
		}
		for _, rid := range ids {
			var one int
			err := q.QueryRowContext(ctx, `
				SELECT 1 FROM wiki_page WHERE id = ? AND parent_id = ? AND campaign_id = ? AND deleted_at IS NULL`,
				rid, target.TargetDatabaseID, campaignID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, invalid("%s: %q is not a record of the target database", what, rid)
			}
			if err != nil {
				return nil, fmt.Errorf("find related record: %w", err)
			}
		}
		return ids, nil

	case PropGameLink:
		list, ok := v.([]any)
		if !ok || len(list) < 1 || len(list) > maxGameLinks {
			return nil, invalid("%s: must be an array of 1 to %d links", what, maxGameLinks)
		}
		out := make([]gameLink, len(list))
		seen := map[gameLink]bool{}
		for i := range list {
			o, ok := list[i].(map[string]any)
			if !ok || len(o) != 2 {
				return nil, invalid("%s: a link must be {kind, id}", what)
			}
			kind, _ := o["kind"].(string)
			target, _ := o["id"].(string)
			l := gameLink{Kind: kind, ID: target}
			if target == "" || seen[l] {
				return nil, invalid("%s: a link must be unique and have an id", what)
			}
			seen[l] = true
			var query string
			switch kind {
			case "scene":
				query = `SELECT 1 FROM scene WHERE id = ? AND campaign_id = ?`
			case "token":
				query = `SELECT 1 FROM token t JOIN scene s ON s.id = t.scene_id WHERE t.id = ? AND s.campaign_id = ?`
			default:
				return nil, invalid("%s: link kind must be scene or token", what)
			}
			var one int
			err := q.QueryRowContext(ctx, query, target, campaignID).Scan(&one)
			if errors.Is(err, sql.ErrNoRows) {
				return nil, invalid("%s: no such %s %q", what, kind, target)
			}
			if err != nil {
				return nil, fmt.Errorf("find linked %s: %w", kind, err)
			}
			out[i] = l
		}
		return out, nil
	}
	return nil, invalid("%s: unknown property type", what)
}

// applyProps merges values into the props of a record of the database dbID
// (a JSON null deletes the key), stores the result and rebuilds the links.
func (s *Service) applyProps(ctx context.Context, q querier, campaignID, dbID, recordID string, values map[string]json.RawMessage) (json.RawMessage, error) {
	props, err := listProperties(ctx, q, dbID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]Property, len(props))
	types := make(map[string]string, len(props))
	for _, p := range props {
		byID[p.ID] = p
		types[p.ID] = p.Type
	}
	var stored string
	if err := q.QueryRowContext(ctx, `SELECT props FROM wiki_page WHERE id = ?`, recordID).Scan(&stored); err != nil {
		return nil, fmt.Errorf("read props: %w", err)
	}
	cur, err := decodeProps(stored)
	if err != nil {
		return nil, err
	}
	for _, key := range slices.Sorted(maps.Keys(values)) {
		p, ok := byID[key]
		if !ok {
			return nil, invalid("props: %q is not a property of the database", key)
		}
		raw := values[key]
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			delete(cur, key)
			continue
		}
		v, err := checkValue(ctx, q, campaignID, p, raw)
		if err != nil {
			return nil, err
		}
		cur[key] = v
	}
	out, err := json.Marshal(cur)
	if err != nil {
		return nil, fmt.Errorf("marshal props: %w", err)
	}
	if _, err := q.ExecContext(ctx, `UPDATE wiki_page SET props = ?, updated_at = ? WHERE id = ?`,
		string(out), s.now().UnixMilli(), recordID); err != nil {
		return nil, fmt.Errorf("update props: %w", err)
	}
	if err := rebuildLinks(ctx, q, recordID, out, types); err != nil {
		return nil, err
	}
	return out, nil
}

// SetProps merges values into a record's props: a JSON null deletes the key.
// A page that is not a database record: ErrInvalid.
func (s *Service) SetProps(ctx context.Context, recordID string, values map[string]json.RawMessage) (json.RawMessage, error) {
	tx, err := s.begin(ctx, "set props")
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	lp, err := loadLive(ctx, tx, recordID)
	if err != nil {
		return nil, err
	}
	if lp.parentKind != KindDatabase {
		return nil, invalid("the page is not a database record")
	}
	out, err := s.applyProps(ctx, tx, lp.campaignID, lp.parentID, recordID, values)
	if err != nil {
		return nil, err
	}
	if err := commit(tx, "set props"); err != nil {
		return nil, err
	}
	return out, nil
}
