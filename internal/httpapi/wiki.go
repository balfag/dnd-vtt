package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/balfag/dnd-vtt/internal/wiki"
	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

// maxPageRequestBytes is room for a document of doc.MaxPageBytes and the other fields.
const maxPageRequestBytes = doc.MaxPageBytes + 64<<10

type pageNodeJSON struct {
	ID          string  `json:"id"`
	ParentID    *string `json:"parentId"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Icon        string  `json:"icon"`
	Position    int     `json:"position"`
	RecordCount int     `json:"recordCount"`
}

type pageRefJSON struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Icon  string `json:"icon"`
	Kind  string `json:"kind"`
}

type tagJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type refInfoJSON struct {
	Label  string `json:"label"`
	Exists bool   `json:"exists"`
}

type pageJSON struct {
	ID           string                 `json:"id"`
	CampaignID   string                 `json:"campaignId"`
	ParentID     *string                `json:"parentId"`
	Kind         string                 `json:"kind"`
	IsRecord     bool                   `json:"isRecord"`
	Title        string                 `json:"title"`
	Icon         string                 `json:"icon"`
	CoverAssetID *string                `json:"coverAssetId"`
	Content      json.RawMessage        `json:"content"`
	Props        json.RawMessage        `json:"props"`
	Tags         []tagJSON              `json:"tags"`
	Path         []pageRefJSON          `json:"path"`
	Refs         map[string]refInfoJSON `json:"refs"`
	Version      int                    `json:"version"`
	CreatedAt    int64                  `json:"createdAt"`
	UpdatedAt    int64                  `json:"updatedAt"`
}

type savedPageJSON struct {
	Version   int                    `json:"version"`
	UpdatedAt int64                  `json:"updatedAt"`
	Refs      map[string]refInfoJSON `json:"refs"`
}

type newPropertyRequest struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"`
	Options json.RawMessage `json:"options"`
}

type newViewRequest struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type newDatabaseRequest struct {
	Properties []newPropertyRequest `json:"properties"`
	Views      []newViewRequest     `json:"views"`
}

type newPageRequest struct {
	ParentID *string             `json:"parentId"`
	Kind     string              `json:"kind"`
	Title    string              `json:"title"`
	Icon     string              `json:"icon"`
	Content  json.RawMessage     `json:"content"`
	Props    json.RawMessage     `json:"props"`
	Database *newDatabaseRequest `json:"database"`
}

type savePageRequest struct {
	Version      int             `json:"version"`
	Title        string          `json:"title"`
	Icon         string          `json:"icon"`
	CoverAssetID *string         `json:"coverAssetId"`
	Content      json.RawMessage `json:"content"`
	Force        bool            `json:"force"`
}

type movePageRequest struct {
	ParentID *string `json:"parentId"`
	Index    int     `json:"index"` // < 0: 400
}

// optional maps "" to JSON null.
func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// orEmpty is the id behind an optional pointer: null and absent mean "".
func orEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func newRefsJSON(refs map[string]wiki.RefInfo) map[string]refInfoJSON {
	out := make(map[string]refInfoJSON, len(refs))
	for k, v := range refs {
		out[k] = refInfoJSON{Label: v.Label, Exists: v.Exists}
	}
	return out
}

func newPageJSON(p wiki.Page) pageJSON {
	tags := make([]tagJSON, len(p.Tags))
	for i, t := range p.Tags {
		tags[i] = tagJSON{ID: t.ID, Name: t.Name}
	}
	path := make([]pageRefJSON, len(p.Path))
	for i, r := range p.Path {
		path[i] = pageRefJSON{ID: r.ID, Title: r.Title, Icon: r.Icon, Kind: r.Kind}
	}
	return pageJSON{
		ID:           p.ID,
		CampaignID:   p.CampaignID,
		ParentID:     optional(p.ParentID),
		Kind:         p.Kind,
		IsRecord:     p.IsRecord,
		Title:        p.Title,
		Icon:         p.Icon,
		CoverAssetID: optional(p.CoverAssetID),
		Content:      p.Content,
		Props:        p.Props,
		Tags:         tags,
		Path:         path,
		Refs:         newRefsJSON(p.Refs),
		Version:      p.Version,
		CreatedAt:    p.CreatedAt.UnixMilli(),
		UpdatedAt:    p.UpdatedAt.UnixMilli(),
	}
}

// wikiError answers with the response matching a wiki.Service error.
func (a *api) wikiError(w http.ResponseWriter, action string, err error) {
	switch {
	case errors.Is(err, wiki.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, wiki.ErrInvalid):
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, wiki.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", err.Error())
	default:
		a.internalError(w, action, err)
	}
}

// badWikiBody answers for a body decodeJSONMax rejected.
func badWikiBody(w http.ResponseWriter, err error) {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "the request is too large")
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request", err.Error())
}

func (a *api) getWikiTree(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	nodes, err := a.d.Wiki.Tree(r.Context(), r.PathValue("id"))
	if err != nil {
		a.wikiError(w, "list wiki tree", err)
		return
	}
	out := make([]pageNodeJSON, len(nodes))
	for i, n := range nodes {
		out[i] = pageNodeJSON{
			ID: n.ID, ParentID: optional(n.ParentID), Kind: n.Kind, Title: n.Title, Icon: n.Icon,
			Position: n.Position, RecordCount: n.RecordCount,
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Pages []pageNodeJSON `json:"pages"`
	}{out})
}

func (a *api) createWikiPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req newPageRequest
	if err := decodeJSONMax(w, r, &req, maxPageRequestBytes); err != nil {
		badWikiBody(w, err)
		return
	}
	content := req.Content
	if bytes.Equal(bytes.TrimSpace(content), []byte("null")) {
		content = nil
	}
	np := wiki.NewPage{
		ParentID: orEmpty(req.ParentID), Kind: req.Kind, Title: req.Title, Icon: req.Icon, Content: content, Props: req.Props,
	}
	if req.Database != nil {
		nd := &wiki.NewDatabase{}
		for _, p := range req.Database.Properties {
			nd.Properties = append(nd.Properties, wiki.NewProperty{Name: p.Name, Type: p.Type, Options: p.Options})
		}
		for _, v := range req.Database.Views {
			nd.Views = append(nd.Views, wiki.NewView{Name: v.Name, Kind: v.Kind})
		}
		np.Database = nd
	}
	page, err := a.d.Wiki.Create(r.Context(), r.PathValue("id"), np)
	if err != nil {
		a.wikiError(w, "create wiki page", err)
		return
	}
	writeJSON(w, http.StatusCreated, newPageJSON(page))
}

func (a *api) getWikiPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	page, err := a.d.Wiki.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		a.wikiError(w, "get wiki page", err)
		return
	}
	writeJSON(w, http.StatusOK, newPageJSON(page))
}

func (a *api) saveWikiPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req savePageRequest
	if err := decodeJSONMax(w, r, &req, maxPageRequestBytes); err != nil {
		badWikiBody(w, err)
		return
	}
	if len(req.Content) == 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "content is required")
		return
	}
	saved, err := a.d.Wiki.Save(r.Context(), r.PathValue("id"), wiki.SavePage{
		Version: req.Version, Title: req.Title, Icon: req.Icon, CoverAssetID: orEmpty(req.CoverAssetID),
		Content: req.Content, Force: req.Force,
	})
	if err != nil {
		a.wikiError(w, "save wiki page", err)
		return
	}
	writeJSON(w, http.StatusOK, savedPageJSON{
		Version: saved.Version, UpdatedAt: saved.UpdatedAt.UnixMilli(), Refs: newRefsJSON(saved.Refs),
	})
}

func (a *api) moveWikiPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req movePageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	if req.Index < 0 {
		writeError(w, http.StatusBadRequest, "bad_request", "index must not be negative")
		return
	}
	if err := a.d.Wiki.Move(r.Context(), r.PathValue("id"), orEmpty(req.ParentID), req.Index); err != nil {
		a.wikiError(w, "move wiki page", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) deleteWikiPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.Delete(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "delete wiki page", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type trashItemJSON struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Icon      string `json:"icon"`
	Kind      string `json:"kind"`
	DeletedAt int64  `json:"deletedAt"`
	PurgeAt   int64  `json:"purgeAt"`
	PageCount int    `json:"pageCount"`
}

type tagCountJSON struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	PageCount int    `json:"pageCount"`
}

type setTagsRequest struct {
	Names []string `json:"names"`
}

type renameTagRequest struct {
	Name  string `json:"name"`
	Merge bool   `json:"merge"`
}

func (a *api) getWikiTrash(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	items, err := a.d.Wiki.Trash(r.Context(), r.PathValue("id"))
	if err != nil {
		a.wikiError(w, "list wiki trash", err)
		return
	}
	out := make([]trashItemJSON, len(items))
	for i, it := range items {
		out[i] = trashItemJSON{
			ID: it.ID, Title: it.Title, Icon: it.Icon, Kind: it.Kind,
			DeletedAt: it.DeletedAt.UnixMilli(), PurgeAt: it.PurgeAt.UnixMilli(), PageCount: it.PageCount,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) restoreWikiTrash(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.Restore(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "restore wiki pages", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) purgeWikiTrash(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.Purge(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "purge wiki pages", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) emptyWikiTrash(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.EmptyTrash(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "empty wiki trash", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) setWikiPageTags(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req setTagsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	tags, err := a.d.Wiki.SetTags(r.Context(), r.PathValue("id"), req.Names)
	if err != nil {
		a.wikiError(w, "set wiki tags", err)
		return
	}
	out := make([]tagJSON, len(tags))
	for i, t := range tags {
		out[i] = tagJSON{ID: t.ID, Name: t.Name}
	}
	writeJSON(w, http.StatusOK, struct {
		Tags []tagJSON `json:"tags"`
	}{out})
}

func (a *api) listWikiTags(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	tags, err := a.d.Wiki.Tags(r.Context(), r.PathValue("id"))
	if err != nil {
		a.wikiError(w, "list wiki tags", err)
		return
	}
	out := make([]tagCountJSON, len(tags))
	for i, t := range tags {
		out[i] = tagCountJSON{ID: t.ID, Name: t.Name, PageCount: t.PageCount}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) renameWikiTag(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req renameTagRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	tag, err := a.d.Wiki.RenameTag(r.Context(), r.PathValue("id"), req.Name, req.Merge)
	if err != nil {
		a.wikiError(w, "rename wiki tag", err)
		return
	}
	writeJSON(w, http.StatusOK, tagJSON{ID: tag.ID, Name: tag.Name})
}

func (a *api) deleteWikiTag(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.DeleteTag(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "delete wiki tag", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type segmentJSON struct {
	Text  string `json:"text"`
	Match bool   `json:"match,omitempty"`
}

type searchHitJSON struct {
	ID       string        `json:"id"`
	Title    string        `json:"title"`
	Icon     string        `json:"icon"`
	Kind     string        `json:"kind"`
	IsRecord bool          `json:"isRecord"`
	Path     []string      `json:"path"`
	Snippet  []segmentJSON `json:"snippet"`
}

type mentionHitJSON struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Label  string `json:"label"`
	Detail string `json:"detail"`
}

func (a *api) searchWiki(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	query := r.URL.Query()
	q := wiki.SearchQuery{Q: query.Get("q"), TagID: query.Get("tag")}
	if raw := query.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 50 {
			writeError(w, http.StatusBadRequest, "bad_request", "limit must be a number from 1 to 50")
			return
		}
		q.Limit = n
	}
	hits, err := a.d.Wiki.Search(r.Context(), r.PathValue("id"), q)
	if err != nil {
		a.wikiError(w, "search wiki", err)
		return
	}
	out := make([]searchHitJSON, len(hits))
	for i, h := range hits {
		segs := make([]segmentJSON, len(h.Snippet))
		for j, s := range h.Snippet {
			segs[j] = segmentJSON{Text: s.Text, Match: s.Match}
		}
		out[i] = searchHitJSON{
			ID: h.ID, Title: h.Title, Icon: h.Icon, Kind: h.Kind, IsRecord: h.IsRecord,
			Path: h.Path, Snippet: segs,
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) mentionSearchWiki(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	hits, err := a.d.Wiki.MentionSearch(r.Context(), r.PathValue("id"), r.URL.Query().Get("q"))
	if err != nil {
		a.wikiError(w, "search wiki mentions", err)
		return
	}
	out := make([]mentionHitJSON, len(hits))
	for i, h := range hits {
		out[i] = mentionHitJSON{Kind: h.Kind, ID: h.ID, Label: h.Label, Detail: h.Detail}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) getWikiBacklinks(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	refs, err := a.d.Wiki.Backlinks(r.Context(), r.PathValue("id"))
	if err != nil {
		a.wikiError(w, "list wiki backlinks", err)
		return
	}
	out := make([]pageRefJSON, len(refs))
	for i, p := range refs {
		out[i] = pageRefJSON{ID: p.ID, Title: p.Title, Icon: p.Icon, Kind: p.Kind}
	}
	writeJSON(w, http.StatusOK, out)
}

type propertyJSON struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Position int             `json:"position"`
	Options  json.RawMessage `json:"options"`
}

type viewJSON struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Kind     string          `json:"kind"`
	Position int             `json:"position"`
	Config   json.RawMessage `json:"config"`
}

type recordJSON struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Icon         string          `json:"icon"`
	CoverAssetID *string         `json:"coverAssetId"`
	Props        json.RawMessage `json:"props"`
	Tags         []tagJSON       `json:"tags"`
	CreatedAt    int64           `json:"createdAt"`
	UpdatedAt    int64           `json:"updatedAt"`
}

type databaseJSON struct {
	Properties []propertyJSON `json:"properties"`
	Views      []viewJSON     `json:"views"`
	Records    []recordJSON   `json:"records"`
}

func newPropertyJSON(p wiki.Property) propertyJSON {
	return propertyJSON{ID: p.ID, Name: p.Name, Type: p.Type, Position: p.Position, Options: p.Options}
}

func newViewJSON(v wiki.View) viewJSON {
	return viewJSON{ID: v.ID, Name: v.Name, Kind: v.Kind, Position: v.Position, Config: v.Config}
}

type patchPropertyRequest struct {
	Name     *string         `json:"name"`
	Options  json.RawMessage `json:"options"`
	Position *int            `json:"position"`
}

type addViewRequest struct {
	Name   string          `json:"name"`
	Kind   string          `json:"kind"`
	Config json.RawMessage `json:"config"`
}

type patchViewRequest struct {
	Name     *string         `json:"name"`
	Config   json.RawMessage `json:"config"`
	Position *int            `json:"position"`
}

type setPropsRequest struct {
	Props map[string]json.RawMessage `json:"props"`
}

// nilIfNull maps a JSON null to nil: "not given".
func nilIfNull(raw json.RawMessage) json.RawMessage {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	return raw
}

func (a *api) getWikiDatabase(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	db, err := a.d.Wiki.Database(r.Context(), r.PathValue("pageId"))
	if err != nil {
		a.wikiError(w, "get wiki database", err)
		return
	}
	out := databaseJSON{
		Properties: make([]propertyJSON, len(db.Properties)),
		Views:      make([]viewJSON, len(db.Views)),
		Records:    make([]recordJSON, len(db.Records)),
	}
	for i, p := range db.Properties {
		out.Properties[i] = newPropertyJSON(p)
	}
	for i, v := range db.Views {
		out.Views[i] = newViewJSON(v)
	}
	for i, rec := range db.Records {
		tags := make([]tagJSON, len(rec.Tags))
		for j, t := range rec.Tags {
			tags[j] = tagJSON{ID: t.ID, Name: t.Name}
		}
		out.Records[i] = recordJSON{
			ID: rec.ID, Title: rec.Title, Icon: rec.Icon, CoverAssetID: optional(rec.CoverAssetID), Props: rec.Props,
			Tags: tags, CreatedAt: rec.CreatedAt.UnixMilli(), UpdatedAt: rec.UpdatedAt.UnixMilli(),
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) addWikiProperty(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req newPropertyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	p, err := a.d.Wiki.AddProperty(r.Context(), r.PathValue("pageId"), wiki.NewProperty{Name: req.Name, Type: req.Type, Options: req.Options})
	if err != nil {
		a.wikiError(w, "add wiki property", err)
		return
	}
	writeJSON(w, http.StatusCreated, newPropertyJSON(p))
}

func (a *api) patchWikiProperty(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req patchPropertyRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	p, err := a.d.Wiki.UpdateProperty(r.Context(), r.PathValue("id"), wiki.PropertyPatch{
		Name: req.Name, Options: nilIfNull(req.Options), Position: req.Position,
	})
	if err != nil {
		a.wikiError(w, "update wiki property", err)
		return
	}
	writeJSON(w, http.StatusOK, newPropertyJSON(p))
}

func (a *api) deleteWikiProperty(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.DeleteProperty(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "delete wiki property", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) addWikiView(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req addViewRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	v, err := a.d.Wiki.AddView(r.Context(), r.PathValue("pageId"), req.Name, req.Kind, nilIfNull(req.Config))
	if err != nil {
		a.wikiError(w, "add wiki view", err)
		return
	}
	writeJSON(w, http.StatusCreated, newViewJSON(v))
}

func (a *api) patchWikiView(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req patchViewRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	v, err := a.d.Wiki.UpdateView(r.Context(), r.PathValue("id"), wiki.ViewPatch{
		Name: req.Name, Config: nilIfNull(req.Config), Position: req.Position,
	})
	if err != nil {
		a.wikiError(w, "update wiki view", err)
		return
	}
	writeJSON(w, http.StatusOK, newViewJSON(v))
}

func (a *api) deleteWikiView(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Wiki.DeleteView(r.Context(), r.PathValue("id")); err != nil {
		a.wikiError(w, "delete wiki view", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) setWikiPageProps(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	var req setPropsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		badWikiBody(w, err)
		return
	}
	if req.Props == nil {
		writeError(w, http.StatusBadRequest, "bad_request", "props is required")
		return
	}
	props, err := a.d.Wiki.SetProps(r.Context(), r.PathValue("id"), req.Props)
	if err != nil {
		a.wikiError(w, "set wiki props", err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Props json.RawMessage `json:"props"`
	}{props})
}
