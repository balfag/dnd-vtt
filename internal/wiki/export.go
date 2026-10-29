package wiki

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode"

	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

const (
	maxFileName  = 100 // code points, without the extension
	untitledName = "Без названия"
	nameColumn   = "Название"
)

// AssetFiles opens asset files for the zip (asset.Service implements it via a small adapter in httpapi).
type AssetFiles interface {
	Open(ctx context.Context, id string) (r io.ReadCloser, ext string, sha256 string, err error)
}

// SafeFileName makes a file or folder name of a title: without \/:*?"<>| and
// control characters, at most 100 code points, no edge spaces or dots.
func SafeFileName(title string) string {
	var b strings.Builder
	n := 0
	for _, r := range strings.ToValidUTF8(title, "") {
		if unicode.IsControl(r) || strings.ContainsRune(`\/:*?"<>|`, r) {
			continue
		}
		if n++; n > maxFileName {
			break
		}
		b.WriteRune(r)
	}
	name := strings.TrimSpace(b.String())
	name = strings.TrimSpace(strings.TrimRight(name, "."))
	if name == "" {
		return untitledName
	}
	return name
}

// exPage is a live page loaded for the export.
type exPage struct {
	id, kind, title, icon string
	content               []byte // empty when the content is not needed
	props                 map[string]any
	tags                  []string
	isRecord              bool
	parent                *exPage
	children              []*exPage
	path                  string // slash-separated path of the file in the archive
}

type exSet struct {
	pages    map[string]*exPage
	roots    []*exPage
	props    map[string][]Property // by database
	view     map[string]View       // the first view by database
	scenes   map[string]string
	tokens   map[string]string
	assetRef map[string]string // asset id -> path in the archive
}

// loadExport reads the live pages of the campaign. The content is read only
// for contentOf ("" = all pages).
func (s *Service) loadExport(ctx context.Context, campaignID, contentOf string) (*exSet, error) {
	set := &exSet{
		pages: map[string]*exPage{}, props: map[string][]Property{}, view: map[string]View{},
		scenes: map[string]string{}, tokens: map[string]string{}, assetRef: map[string]string{},
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(parent_id, ''), kind, title, icon,
		  CASE WHEN ?1 = '' OR id = ?1 THEN content ELSE '' END, props
		FROM wiki_page WHERE campaign_id = ?2 AND deleted_at IS NULL
		ORDER BY COALESCE(parent_id, ''), position, id`, contentOf, campaignID)
	if err != nil {
		return nil, fmt.Errorf("load wiki pages: %w", err)
	}
	defer func() { _ = rows.Close() }()
	parents := map[string]string{}
	var order []*exPage
	for rows.Next() {
		var (
			p              exPage
			parent         string
			content, props string
		)
		if err := rows.Scan(&p.id, &parent, &p.kind, &p.title, &p.icon, &content, &props); err != nil {
			return nil, fmt.Errorf("scan wiki page: %w", err)
		}
		p.content = []byte(content)
		if p.props, err = decodeProps(props); err != nil {
			return nil, err
		}
		p.tags = []string{}
		set.pages[p.id] = &p
		parents[p.id] = parent
		order = append(order, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load wiki pages: %w", err)
	}
	for _, p := range order {
		if parent, ok := set.pages[parents[p.id]]; ok {
			parent.children = append(parent.children, p)
			p.parent = parent
			p.isRecord = parent.kind == KindDatabase
		} else {
			set.roots = append(set.roots, p)
		}
	}

	if err := s.loadExportTags(ctx, campaignID, set); err != nil {
		return nil, err
	}
	if err := s.loadExportDatabases(ctx, campaignID, set); err != nil {
		return nil, err
	}
	if err := s.loadExportNames(ctx, campaignID, set); err != nil {
		return nil, err
	}
	return set, nil
}

func (s *Service) loadExportTags(ctx context.Context, campaignID string, set *exSet) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT pt.page_id, t.name FROM wiki_page_tag pt
		JOIN wiki_tag t ON t.id = pt.tag_id JOIN wiki_page p ON p.id = pt.page_id
		WHERE p.campaign_id = ? AND p.deleted_at IS NULL ORDER BY t.name, t.id`, campaignID)
	if err != nil {
		return fmt.Errorf("load page tags: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var pageID, name string
		if err := rows.Scan(&pageID, &name); err != nil {
			return fmt.Errorf("scan page tag: %w", err)
		}
		if p, ok := set.pages[pageID]; ok {
			p.tags = append(p.tags, name)
		}
	}
	return rows.Err()
}

func (s *Service) loadExportDatabases(ctx context.Context, campaignID string, set *exSet) error {
	for _, p := range set.pages {
		if p.kind != KindDatabase {
			continue
		}
		props, err := listProperties(ctx, s.db, p.id)
		if err != nil {
			return err
		}
		set.props[p.id] = props
		views, err := listViews(ctx, s.db, p.id)
		if err != nil {
			return err
		}
		if len(views) > 0 {
			set.view[p.id] = views[0]
		}
	}
	return nil
}

func (s *Service) loadExportNames(ctx context.Context, campaignID string, set *exSet) error {
	for _, q := range []struct {
		query string
		dst   map[string]string
	}{
		{`SELECT id, name FROM scene WHERE campaign_id = ?`, set.scenes},
		{`SELECT t.id, t.name FROM token t JOIN scene s ON s.id = t.scene_id WHERE s.campaign_id = ?`, set.tokens},
	} {
		rows, err := s.db.QueryContext(ctx, q.query, campaignID)
		if err != nil {
			return fmt.Errorf("load game names: %w", err)
		}
		for rows.Next() {
			var id, name string
			if err := rows.Scan(&id, &name); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan game name: %w", err)
			}
			q.dst[id] = name
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return fmt.Errorf("load game names: %w", err)
		}
	}
	return nil
}

// ---- values of the front matter and of the table of a database ----

// rawValue is a YAML scalar written as is: a number or a boolean.
type rawValue string

// propValue returns the value of property p of a record: a string, a rawValue
// or a []string; ok=false: no value.
func (set *exSet) propValue(p Property, v any) (any, bool) {
	switch p.Type {
	case PropText, PropURL, PropDate:
		s, ok := v.(string)
		return s, ok && s != ""
	case PropNumber:
		n, ok := v.(json.Number)
		return rawValue(n), ok
	case PropCheckbox:
		b, ok := v.(bool)
		return rawValue(fmt.Sprint(b)), ok
	case PropSelect:
		sid, _ := v.(string)
		name, ok := optionName(p, sid)
		return name, ok
	case PropMultiSelect:
		var out []string
		list, _ := v.([]any)
		for _, x := range list {
			sid, _ := x.(string)
			if name, ok := optionName(p, sid); ok {
				out = append(out, name)
			}
		}
		return out, len(out) > 0
	case PropRelation:
		var out []string
		list, _ := v.([]any)
		for _, x := range list {
			rid, _ := x.(string)
			if rec, ok := set.pages[rid]; ok {
				out = append(out, rec.title)
			}
		}
		return out, len(out) > 0
	case PropGameLink:
		var out []string
		list, _ := v.([]any)
		for _, x := range list {
			o, _ := x.(map[string]any)
			kind, _ := o["kind"].(string)
			gid, _ := o["id"].(string)
			names := set.scenes
			if kind == "token" {
				names = set.tokens
			}
			if name, ok := names[gid]; ok {
				out = append(out, name)
			}
		}
		return out, len(out) > 0
	}
	return nil, false
}

func optionName(p Property, optionID string) (string, bool) {
	var so selectOptions
	if err := json.Unmarshal(p.Options, &so); err != nil {
		return "", false
	}
	for _, o := range so.Options {
		if o.ID == optionID {
			return o.Name, true
		}
	}
	return "", false
}

// recordValues returns the present values of the record in the order of the properties.
func (set *exSet) recordValues(rec *exPage, props []Property) map[string]any {
	out := map[string]any{}
	for _, p := range props {
		if v, ok := set.propValue(p, rec.props[p.ID]); ok {
			out[p.ID] = v
		}
	}
	return out
}

// yamlString quotes s; unlike strconv.Quote it keeps non-ASCII text as is.
func yamlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range strings.ToValidUTF8(s, "�") {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\x%02X`, r)
		case r == 0x85 || r == 0x2028 || r == 0x2029:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func yamlValue(v any) string {
	switch v := v.(type) {
	case rawValue:
		return string(v)
	case []string:
		q := make([]string, len(v))
		for i, s := range v {
			q[i] = yamlString(s)
		}
		return "[" + strings.Join(q, ", ") + "]"
	}
	return yamlString(fmt.Sprint(v))
}

func plainValue(v any) string {
	switch v := v.(type) {
	case rawValue:
		return string(v)
	case []string:
		return strings.Join(v, ", ")
	}
	return fmt.Sprint(v)
}

func (set *exSet) frontMatter(p *exPage) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlString(p.title) + "\n")
	b.WriteString("icon: " + yamlString(p.icon) + "\n")
	b.WriteString("tags: " + yamlValue(p.tags) + "\n")
	if p.isRecord {
		props := set.props[p.parent.id]
		values := set.recordValues(p, props)
		if len(values) > 0 {
			b.WriteString("props:\n")
			for _, pr := range props {
				if v, ok := values[pr.ID]; ok {
					b.WriteString("  " + yamlString(pr.Name) + ": " + yamlValue(v) + "\n")
				}
			}
		}
	}
	b.WriteString("---\n")
	return b.String()
}

// databaseTable renders the records of the database as a GFM table.
func (set *exSet) databaseTable(db *exPage, l *exportLinker) string {
	props := set.props[db.id]
	cols := props
	if v, ok := set.view[db.id]; ok {
		var cfg viewConfig
		if json.Unmarshal(v.Config, &cfg) == nil {
			byID := map[string]Property{}
			for _, p := range props {
				byID[p.ID] = p
			}
			cols = nil
			for _, pid := range cfg.Visible {
				if p, ok := byID[pid]; ok {
					cols = append(cols, p)
				}
			}
		}
	}
	esc := &mdRenderer{inTable: true}
	cell := func(s string) string {
		return " " + strings.ReplaceAll(esc.escape(strings.ReplaceAll(s, "\n", " "), false), "\r", "") + " |"
	}
	head, sep := "|"+cell(nameColumn), "| --- |"
	for _, p := range cols {
		head += cell(p.Name)
		sep += " --- |"
	}
	lines := []string{head, sep}
	for _, rec := range db.children {
		title := esc.escape(strings.ReplaceAll(rec.title, "\n", " "), false)
		if href, ok := l.Page(rec.id); ok {
			title = "[" + title + "](" + href + ")"
		}
		line := "| " + title + " |"
		values := set.recordValues(rec, props)
		for _, p := range cols {
			s := ""
			if v, ok := values[p.ID]; ok {
				s = plainValue(v)
			}
			line += cell(s)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// ---- links ----

// exportLinker resolves the links of the file at fromDir.
type exportLinker struct {
	set     *exSet
	fromDir string // "" or "A/B/"
	links   bool   // false: links to pages become text, assets are /assets/<id>
}

func (l *exportLinker) Page(id string) (string, bool) {
	if !l.links {
		return "", false
	}
	p, ok := l.set.pages[id]
	if !ok || p.path == "" {
		return "", false
	}
	return relPath(l.fromDir, p.path), true
}

func (l *exportLinker) Asset(id string) string {
	if ref, ok := l.set.assetRef[id]; l.links && ok {
		return strings.Repeat("../", strings.Count(l.fromDir, "/")) + ref
	}
	return "/assets/" + id
}

func (l *exportLinker) Label(r doc.Ref, stored string) string {
	var (
		name string
		ok   bool
	)
	switch r.Kind {
	case doc.KindPage:
		var p *exPage
		if p, ok = l.set.pages[r.ID]; ok {
			name = p.title
		}
	case doc.KindScene:
		name, ok = l.set.scenes[r.ID]
	case doc.KindToken:
		name, ok = l.set.tokens[r.ID]
	}
	if !ok {
		return stored
	}
	return name
}

// relPath is the link from the directory fromDir to the file to.
func relPath(fromDir, to string) string {
	var from []string
	if fromDir != "" {
		from = strings.Split(strings.TrimSuffix(fromDir, "/"), "/")
	}
	dst := strings.Split(to, "/")
	c := 0
	for c < len(from) && c < len(dst)-1 && from[c] == dst[c] {
		c++
	}
	parts := make([]string, 0, len(from)-c+len(dst)-c)
	for range from[c:] {
		parts = append(parts, "..")
	}
	for _, seg := range dst[c:] {
		parts = append(parts, linkSegment(seg))
	}
	return strings.Join(parts, "/")
}

func linkSegment(s string) string {
	return strings.NewReplacer(
		"%", "%25", " ", "%20", "(", "%28", ")", "%29", "#", "%23", "?", "%3F", "<", "%3C", ">", "%3E", "[", "%5B", "]", "%5D",
	).Replace(s)
}

// layout gives every page its path in the archive.
func layout(pages []*exPage, dir string) {
	used := map[string]bool{}
	for _, p := range pages {
		base := SafeFileName(p.title)
		name := base
		for n := 2; used[strings.ToLower(name)]; n++ {
			name = fmt.Sprintf("%s (%d)", base, n)
		}
		used[strings.ToLower(name)] = true
		p.path = dir + name + ".md"
		layout(p.children, dir+name+"/")
	}
}

// ---- files ----

// file renders the Markdown file of the page.
func (set *exSet) file(p *exPage, l *exportLinker) (string, error) {
	var b strings.Builder
	b.WriteString(set.frontMatter(p))
	b.WriteString("\n# " + (&mdRenderer{}).escape(strings.ReplaceAll(p.title, "\n", " "), false) + "\n")
	var body string
	if p.kind == KindDatabase {
		body = set.databaseTable(p, l)
	} else {
		var err error
		if body, err = ToMarkdown(p.content, l); err != nil {
			return "", fmt.Errorf("page %s: %w", p.id, err)
		}
	}
	if body != "" {
		b.WriteString("\n" + body + "\n")
	}
	return b.String(), nil
}

// ExportPage returns the file name and the Markdown of one live page.
func (s *Service) ExportPage(ctx context.Context, id string) (name string, md []byte, err error) {
	var campaignID string
	err = s.db.QueryRowContext(ctx, `SELECT campaign_id FROM wiki_page WHERE id = ? AND deleted_at IS NULL`, id).Scan(&campaignID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, ErrNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("find wiki page: %w", err)
	}
	set, err := s.loadExport(ctx, campaignID, id)
	if err != nil {
		return "", nil, err
	}
	p, ok := set.pages[id]
	if !ok {
		return "", nil, ErrNotFound
	}
	out, err := set.file(p, &exportLinker{set: set})
	if err != nil {
		return "", nil, err
	}
	return SafeFileName(p.title) + ".md", []byte(out), nil
}

// ExportAll writes a zip of all live pages of the campaign to w.
func (s *Service) ExportAll(ctx context.Context, campaignID string, w io.Writer, assets AssetFiles) error {
	if err := campaignExists(ctx, s.db, campaignID); err != nil {
		return err
	}
	set, err := s.loadExport(ctx, campaignID, "")
	if err != nil {
		return err
	}
	layout(set.roots, "")
	assetIDs, err := s.exportAssetIDs(ctx, campaignID)
	if err != nil {
		return err
	}
	if len(assetIDs) > 0 && assets == nil {
		return errors.New("export wiki: pages have images but no asset files are given")
	}

	modified := s.now()
	zw := zip.NewWriter(w)
	written := map[string]bool{}
	for _, aid := range assetIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		ref, err := writeAsset(ctx, zw, assets, aid, written, modified)
		if err != nil {
			return err
		}
		set.assetRef[aid] = ref
	}

	var write func(pages []*exPage) error
	write = func(pages []*exPage) error {
		for _, p := range pages {
			if err := ctx.Err(); err != nil {
				return err
			}
			dir := ""
			if i := strings.LastIndex(p.path, "/"); i >= 0 {
				dir = p.path[:i+1]
			}
			md, err := set.file(p, &exportLinker{set: set, fromDir: dir, links: true})
			if err != nil {
				return err
			}
			f, err := zw.CreateHeader(&zip.FileHeader{Name: p.path, Method: zip.Deflate, Modified: modified})
			if err != nil {
				return fmt.Errorf("write %s: %w", p.path, err)
			}
			if _, err := io.WriteString(f, md); err != nil {
				return fmt.Errorf("write %s: %w", p.path, err)
			}
			if err := write(p.children); err != nil {
				return err
			}
		}
		return nil
	}
	if err := write(set.roots); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish archive: %w", err)
	}
	return nil
}

// exportAssetIDs lists the images used by the live pages of the campaign.
func (s *Service) exportAssetIDs(ctx context.Context, campaignID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT pa.asset_id FROM wiki_page_asset pa JOIN wiki_page p ON p.id = pa.page_id
		WHERE p.campaign_id = ? AND p.deleted_at IS NULL ORDER BY pa.asset_id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list page assets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var aid string
		if err := rows.Scan(&aid); err != nil {
			return nil, fmt.Errorf("scan page asset: %w", err)
		}
		out = append(out, aid)
	}
	return out, rows.Err()
}

// writeAsset puts the file of the asset into the archive once and returns its path there.
func writeAsset(ctx context.Context, zw *zip.Writer, assets AssetFiles, assetID string, written map[string]bool, modified time.Time) (string, error) {
	r, ext, sum, err := assets.Open(ctx, assetID)
	if err != nil {
		return "", fmt.Errorf("open asset %s: %w", assetID, err)
	}
	defer func() { _ = r.Close() }()
	name := "assets/" + sum + "." + ext
	if written[name] {
		return name, nil
	}
	written[name] = true
	f, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store, Modified: modified})
	if err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	if _, err := io.Copy(f, r); err != nil {
		return "", fmt.Errorf("write %s: %w", name, err)
	}
	return name, nil
}