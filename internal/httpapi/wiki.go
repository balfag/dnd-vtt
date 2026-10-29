package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

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

type newPageRequest struct {
	ParentID *string         `json:"parentId"`
	Title    string          `json:"title"`
	Icon     string          `json:"icon"`
	Content  json.RawMessage `json:"content"`
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
	page, err := a.d.Wiki.Create(r.Context(), r.PathValue("id"), wiki.NewPage{
		ParentID: orEmpty(req.ParentID), Title: req.Title, Icon: req.Icon, Content: content,
	})
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
