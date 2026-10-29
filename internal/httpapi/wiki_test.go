package httpapi

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/asset"
)

const emptyPageDoc = `{"type":"doc","content":[{"type":"paragraph"}]}`

type wikiFixture struct {
	*apiFixture
	gm         *http.Cookie
	campaignID string
}

func newWikiFixture(t *testing.T) *wikiFixture {
	t.Helper()
	f := newAPIFixture(t)
	gm := f.gmCookie()
	return &wikiFixture{apiFixture: f, gm: gm, campaignID: f.createCampaign(gm, "Camp")}
}

func (f *wikiFixture) createPage(body string) pageJSON {
	f.t.Helper()
	rec := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/wiki/pages", body, withCookie(f.gm))
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("create page = %d %s", rec.Code, rec.Body.String())
	}
	var p pageJSON
	decodeBody(f.t, rec, &p)
	return p
}

func TestWikiRequiresGM(t *testing.T) {
	f := newWikiFixture(t)
	page := f.createPage(`{"title":"A"}`)
	trashed := f.createPage(`{"title":"T"}`)
	if rec := f.do(http.MethodDelete, "/api/wiki/pages/"+trashed.ID, "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	rec := f.do(http.MethodPut, "/api/wiki/pages/"+page.ID+"/tags", `{"names":["x"]}`, withCookie(f.gm))
	var set struct{ Tags []tagJSON }
	decodeBody(t, rec, &set)
	if rec.Code != http.StatusOK || len(set.Tags) != 1 {
		t.Fatalf("set tags = %d", rec.Code)
	}
	rec = f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/wiki/templates", `{"fromPageId":"`+page.ID+`","name":"T"}`, withCookie(f.gm))
	var tpl templateJSON
	decodeBody(t, rec, &tpl)
	if rec.Code != http.StatusCreated {
		t.Fatalf("save template = %d", rec.Code)
	}
	_, link := f.startGame(f.gm, f.campaignID)
	player, _ := f.playerSession(link, "bob")

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/campaigns/" + f.campaignID + "/wiki/tree", ""},
		{http.MethodPost, "/api/campaigns/" + f.campaignID + "/wiki/pages", `{"title":"B"}`},
		{http.MethodGet, "/api/wiki/pages/" + page.ID, ""},
		{http.MethodPut, "/api/wiki/pages/" + page.ID, `{"version":1,"title":"A","content":` + emptyPageDoc + `}`},
		{http.MethodPost, "/api/wiki/pages/" + page.ID + "/move", `{"parentId":null,"index":0}`},
		{http.MethodDelete, "/api/wiki/pages/" + page.ID, ""},
		{http.MethodGet, "/api/campaigns/" + f.campaignID + "/wiki/trash", ""},
		{http.MethodPost, "/api/wiki/trash/" + trashed.ID + "/restore", ""},
		{http.MethodDelete, "/api/wiki/trash/" + trashed.ID, ""},
		{http.MethodDelete, "/api/campaigns/" + f.campaignID + "/wiki/trash", ""},
		{http.MethodPut, "/api/wiki/pages/" + page.ID + "/tags", `{"names":["y"]}`},
		{http.MethodGet, "/api/campaigns/" + f.campaignID + "/wiki/tags", ""},
		{http.MethodPatch, "/api/wiki/tags/" + set.Tags[0].ID, `{"name":"z"}`},
		{http.MethodDelete, "/api/wiki/tags/" + set.Tags[0].ID, ""},
		{http.MethodGet, "/api/campaigns/" + f.campaignID + "/wiki/templates", ""},
		{http.MethodPost, "/api/campaigns/" + f.campaignID + "/wiki/templates", `{"fromPageId":"` + page.ID + `","name":"T"}`},
		{http.MethodPatch, "/api/wiki/templates/" + tpl.ID, `{"name":"U"}`},
		{http.MethodDelete, "/api/wiki/templates/" + tpl.ID, ""},
		{http.MethodGet, "/api/wiki/pages/" + page.ID + "/export.md", ""},
		{http.MethodGet, "/api/campaigns/" + f.campaignID + "/wiki/export.zip", ""},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			if rec := f.do(rt.method, rt.path, rt.body, withCookie(player)); rec.Code != http.StatusForbidden {
				t.Errorf("player = %d, want 403", rec.Code)
			}
			if rec := f.do(rt.method, rt.path, rt.body); rec.Code != http.StatusUnauthorized {
				t.Errorf("anonymous = %d, want 401", rec.Code)
			}
		})
	}
}

func TestWikiPageLifecycle(t *testing.T) {
	f := newWikiFixture(t)

	a := f.createPage(`{"title":"Tavern","icon":"рџЌє"}`)
	if a.Version != 1 || a.Kind != "page" || a.ParentID != nil || a.CoverAssetID != nil || a.CampaignID != f.campaignID ||
		a.Tags == nil || a.Path == nil || a.Refs == nil || string(a.Props) != "{}" || a.CreatedAt == 0 {
		t.Errorf("created = %+v", a)
	}
	b := f.createPage(`{"title":"Cellar","parentId":"` + a.ID + `"}`)
	if b.ParentID == nil || *b.ParentID != a.ID || len(b.Path) != 1 || b.Path[0].ID != a.ID {
		t.Errorf("child = %+v", b)
	}

	var tree struct {
		Pages []pageNodeJSON `json:"pages"`
	}
	rec := f.do(http.MethodGet, "/api/campaigns/"+f.campaignID+"/wiki/tree", "", withCookie(f.gm))
	decodeBody(t, rec, &tree)
	if rec.Code != http.StatusOK || len(tree.Pages) != 2 || tree.Pages[0].ParentID != nil || tree.Pages[1].ParentID == nil {
		t.Errorf("tree = %d %+v", rec.Code, tree)
	}

	rec = f.do(http.MethodGet, "/api/wiki/pages/"+a.ID, "", withCookie(f.gm))
	var got pageJSON
	decodeBody(t, rec, &got)
	if rec.Code != http.StatusOK || got.ID != a.ID || got.Title != "Tavern" {
		t.Errorf("get = %d %+v", rec.Code, got)
	}

	mention := `{"type":"mention","attrs":{"kind":"page","id":"` + b.ID + `"}}`
	content := `{"type":"doc","content":[{"type":"paragraph","content":[` + mention + `]}]}`
	rec = f.do(http.MethodPut, "/api/wiki/pages/"+a.ID, `{"version":1,"title":"Inn","icon":"","content":`+content+`}`, withCookie(f.gm))
	var saved savedPageJSON
	decodeBody(t, rec, &saved)
	if rec.Code != http.StatusOK || saved.Version != 2 || saved.UpdatedAt == 0 || saved.Refs["page:"+b.ID] != (refInfoJSON{Label: "Cellar", Exists: true}) {
		t.Errorf("save = %d %+v", rec.Code, saved)
	}
	rec = f.do(http.MethodPut, "/api/wiki/pages/"+a.ID, `{"version":1,"title":"Inn","content":`+content+`}`, withCookie(f.gm))
	if rec.Code != http.StatusConflict {
		t.Errorf("stale save = %d, want 409", rec.Code)
	}
	rec = f.do(http.MethodPut, "/api/wiki/pages/"+a.ID, `{"version":1,"title":"Inn","content":`+content+`,"force":true}`, withCookie(f.gm))
	if rec.Code != http.StatusOK {
		t.Errorf("forced save = %d %s", rec.Code, rec.Body.String())
	}

	rec = f.do(http.MethodPost, "/api/wiki/pages/"+b.ID+"/move", `{"parentId":null,"index":0}`, withCookie(f.gm))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("move = %d %s", rec.Code, rec.Body.String())
	}
	rec = f.do(http.MethodGet, "/api/wiki/pages/"+b.ID, "", withCookie(f.gm))
	decodeBody(t, rec, &got)
	if got.ParentID != nil || len(got.Path) != 0 {
		t.Errorf("moved page = %+v", got)
	}
	rec = f.do(http.MethodPost, "/api/wiki/pages/"+a.ID+"/move", `{"parentId":"`+a.ID+`","index":0}`, withCookie(f.gm))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("move into itself = %d, want 400", rec.Code)
	}

	rec = f.do(http.MethodDelete, "/api/wiki/pages/"+a.ID, "", withCookie(f.gm))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	rec = f.do(http.MethodGet, "/api/wiki/pages/"+a.ID, "", withCookie(f.gm))
	if rec.Code != http.StatusNotFound {
		t.Errorf("get of a deleted page = %d, want 404", rec.Code)
	}
}

func TestWikiBadRequests(t *testing.T) {
	f := newWikiFixture(t)
	page := f.createPage(`{"title":"A"}`)
	pages := "/api/campaigns/" + f.campaignID + "/wiki/pages"
	one := "/api/wiki/pages/" + page.ID

	tests := []struct {
		name, method, path, body string
		want                     int
	}{
		{"put without content", http.MethodPut, one, `{"version":1,"title":"A"}`, http.StatusBadRequest},
		{"put with an invalid document", http.MethodPut, one, `{"version":1,"title":"A","content":{"type":"x"}}`, http.StatusBadRequest},
		{"post with an unknown kind", http.MethodPost, pages, `{"kind":"folder"}`, http.StatusBadRequest},
		{"post with props on a page", http.MethodPost, pages, `{"props":{"x":1}}`, http.StatusBadRequest},
		{"post with an unknown parent", http.MethodPost, pages, `{"parentId":"01HZX3K9M2N4P6Q8R0S1T2V3W4"}`, http.StatusBadRequest},
		{"post too large", http.MethodPost, pages, `{"title":"` + strings.Repeat("a", 1<<20+64<<10) + `"}`, http.StatusRequestEntityTooLarge},
		{"put too large", http.MethodPut, one, `{"version":1,"title":"` + strings.Repeat("a", 1<<20+64<<10) + `"}`, http.StatusRequestEntityTooLarge},
		{"move with a negative index", http.MethodPost, one + "/move", `{"parentId":null,"index":-1}`, http.StatusBadRequest},
		{"move of an unknown page", http.MethodPost, "/api/wiki/pages/nope/move", `{"parentId":null,"index":0}`, http.StatusNotFound},
		{"tree of an unknown campaign", http.MethodGet, "/api/campaigns/nope/wiki/tree", "", http.StatusNotFound},
		{"post to an unknown campaign", http.MethodPost, "/api/campaigns/nope/wiki/pages", `{}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.do(tt.method, tt.path, tt.body, withCookie(f.gm))
			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d (%.200s)", rec.Code, tt.want, rec.Body.String())
			}
		})
	}
}

func TestWikiTrashCycle(t *testing.T) {
	f := newWikiFixture(t)
	camp := "/api/campaigns/" + f.campaignID
	a := f.createPage(`{"title":"A"}`)
	f.createPage(`{"title":"B","parentId":"` + a.ID + `"}`)

	if rec := f.do(http.MethodDelete, "/api/wiki/pages/"+a.ID, "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	var items []trashItemJSON
	rec := f.do(http.MethodGet, camp+"/wiki/trash", "", withCookie(f.gm))
	decodeBody(t, rec, &items)
	if rec.Code != http.StatusOK || len(items) != 1 || items[0].ID != a.ID || items[0].PageCount != 2 ||
		items[0].Kind != "page" || items[0].PurgeAt-items[0].DeletedAt != 30*24*60*60*1000 {
		t.Fatalf("trash = %d %+v", rec.Code, items)
	}
	if rec := f.do(http.MethodPost, "/api/wiki/trash/nope/restore", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("restore of an unknown id = %d, want 404", rec.Code)
	}
	if rec := f.do(http.MethodPost, "/api/wiki/trash/"+a.ID+"/restore", "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("restore = %d", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/wiki/pages/"+a.ID, "", withCookie(f.gm)); rec.Code != http.StatusOK {
		t.Errorf("get of a restored page = %d", rec.Code)
	}
	rec = f.do(http.MethodGet, camp+"/wiki/trash", "", withCookie(f.gm))
	if rec.Body.String() != "[]\n" && rec.Body.String() != "[]" {
		t.Errorf("empty trash = %q", rec.Body.String())
	}

	f.do(http.MethodDelete, "/api/wiki/pages/"+a.ID, "", withCookie(f.gm))
	if rec := f.do(http.MethodDelete, "/api/wiki/trash/"+a.ID, "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("purge = %d", rec.Code)
	}
	if rec := f.do(http.MethodDelete, "/api/wiki/trash/"+a.ID, "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("second purge = %d, want 404", rec.Code)
	}

	c := f.createPage(`{"title":"C"}`)
	f.do(http.MethodDelete, "/api/wiki/pages/"+c.ID, "", withCookie(f.gm))
	if rec := f.do(http.MethodDelete, camp+"/wiki/trash", "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("empty = %d", rec.Code)
	}
	rec = f.do(http.MethodGet, camp+"/wiki/trash", "", withCookie(f.gm))
	items = nil
	decodeBody(t, rec, &items)
	if len(items) != 0 {
		t.Errorf("trash after empty = %+v", items)
	}
	if rec := f.do(http.MethodGet, "/api/campaigns/nope/wiki/trash", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown campaign = %d, want 404", rec.Code)
	}
}

func TestWikiSearchRequiresGM(t *testing.T) {
	f := newWikiFixture(t)
	page := f.createPage(`{"title":"A"}`)
	_, link := f.startGame(f.gm, f.campaignID)
	player, _ := f.playerSession(link, "bob")

	for _, path := range []string{
		"/api/campaigns/" + f.campaignID + "/wiki/search?q=a",
		"/api/campaigns/" + f.campaignID + "/wiki/mention-search?q=a",
		"/api/wiki/pages/" + page.ID + "/backlinks",
	} {
		if rec := f.do(http.MethodGet, path, "", withCookie(player)); rec.Code != http.StatusForbidden {
			t.Errorf("%s: player = %d, want 403", path, rec.Code)
		}
		if rec := f.do(http.MethodGet, path, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: anonymous = %d, want 401", path, rec.Code)
		}
	}
}

func TestWikiSearch(t *testing.T) {
	f := newWikiFixture(t)
	camp := "/api/campaigns/" + f.campaignID
	page := f.createPage(`{"title":"Таверна"}`)
	content := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Старый маяк на скале"}]}]}`
	if rec := f.do(http.MethodPut, "/api/wiki/pages/"+page.ID, `{"version":1,"title":"Таверна","icon":"","content":`+content+`}`, withCookie(f.gm)); rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}

	rec := f.do(http.MethodGet, camp+"/wiki/search?q=%D0%BC%D0%B0%D1%8F%D0%BA&limit=5", "", withCookie(f.gm))
	if rec.Code != http.StatusOK {
		t.Fatalf("search = %d %s", rec.Code, rec.Body.String())
	}
	var raw []struct {
		ID      string                       `json:"id"`
		Path    []string                     `json:"path"`
		Snippet []map[string]json.RawMessage `json:"snippet"`
	}
	decodeBody(t, rec, &raw)
	if len(raw) != 1 || raw[0].ID != page.ID || raw[0].Path == nil || len(raw[0].Snippet) != 3 {
		t.Fatalf("hits = %s", rec.Body.String())
	}
	for i, seg := range raw[0].Snippet {
		_, hasMatch := seg["match"]
		if hasMatch != (i == 1) || len(seg["text"]) == 0 {
			t.Errorf("segment %d = %v", i, seg)
		}
	}

	for _, q := range []string{"?q=a&limit=0", "?q=a&limit=51", "?q=a&limit=x", "?q=" + strings.Repeat("a", 201), "", "?tag=nope"} {
		want := http.StatusBadRequest
		if q == "?tag=nope" {
			want = http.StatusNotFound
		}
		if rec := f.do(http.MethodGet, camp+"/wiki/search"+q, "", withCookie(f.gm)); rec.Code != want {
			t.Errorf("search%s = %d, want %d", q, rec.Code, want)
		}
	}
}

func TestWikiMentionSearchAndBacklinks(t *testing.T) {
	f := newWikiFixture(t)
	camp := "/api/campaigns/" + f.campaignID
	target := f.createPage(`{"title":"Гавань"}`)
	src := f.createPage(`{"title":"Порт"}`)
	mention := `{"type":"mention","attrs":{"kind":"page","id":"` + target.ID + `"}}`
	content := `{"type":"doc","content":[{"type":"paragraph","content":[` + mention + `]}]}`
	if rec := f.do(http.MethodPut, "/api/wiki/pages/"+src.ID, `{"version":1,"title":"Порт","icon":"","content":`+content+`}`, withCookie(f.gm)); rec.Code != http.StatusOK {
		t.Fatalf("save = %d %s", rec.Code, rec.Body.String())
	}

	var hits []map[string]string
	rec := f.do(http.MethodGet, camp+"/wiki/mention-search?q=%D0%B3%D0%B0%D0%B2", "", withCookie(f.gm))
	decodeBody(t, rec, &hits)
	if rec.Code != http.StatusOK || len(hits) != 1 || hits[0]["kind"] != "page" || hits[0]["id"] != target.ID ||
		hits[0]["label"] != "Гавань" || hits[0]["detail"] != "" {
		t.Errorf("mention-search = %d %v", rec.Code, hits)
	}
	if rec := f.do(http.MethodGet, camp+"/wiki/mention-search?q="+strings.Repeat("a", 101), "", withCookie(f.gm)); rec.Code != http.StatusBadRequest {
		t.Errorf("long q = %d, want 400", rec.Code)
	}

	var refs []pageRefJSON
	rec = f.do(http.MethodGet, "/api/wiki/pages/"+target.ID+"/backlinks", "", withCookie(f.gm))
	decodeBody(t, rec, &refs)
	if rec.Code != http.StatusOK || len(refs) != 1 || refs[0].ID != src.ID || refs[0].Title != "Порт" {
		t.Errorf("backlinks = %d %+v", rec.Code, refs)
	}
	if rec := f.do(http.MethodGet, "/api/wiki/pages/nope/backlinks", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown page = %d, want 404", rec.Code)
	}
}

func TestWikiTags(t *testing.T) {
	f := newWikiFixture(t)
	camp := "/api/campaigns/" + f.campaignID
	a := f.createPage(`{"title":"A"}`)
	b := f.createPage(`{"title":"B"}`)

	var set struct {
		Tags []tagJSON `json:"tags"`
	}
	rec := f.do(http.MethodPut, "/api/wiki/pages/"+a.ID+"/tags", `{"names":["Морвен","город"]}`, withCookie(f.gm))
	decodeBody(t, rec, &set)
	if rec.Code != http.StatusOK || len(set.Tags) != 2 || set.Tags[0].Name != "город" {
		t.Fatalf("set = %d %+v", rec.Code, set)
	}
	if rec := f.do(http.MethodPut, "/api/wiki/pages/"+b.ID+"/tags", `{"names":["МОРВЕН"]}`, withCookie(f.gm)); rec.Code != http.StatusOK {
		t.Fatalf("set b = %d", rec.Code)
	}
	if rec := f.do(http.MethodPut, "/api/wiki/pages/"+a.ID+"/tags", `{"names":[" "]}`, withCookie(f.gm)); rec.Code != http.StatusBadRequest {
		t.Errorf("empty name = %d, want 400", rec.Code)
	}
	if rec := f.do(http.MethodPut, "/api/wiki/pages/nope/tags", `{"names":["x"]}`, withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown page = %d, want 404", rec.Code)
	}

	var tags []tagCountJSON
	rec = f.do(http.MethodGet, camp+"/wiki/tags", "", withCookie(f.gm))
	decodeBody(t, rec, &tags)
	if rec.Code != http.StatusOK || len(tags) != 2 || tags[0].Name != "город" || tags[0].PageCount != 1 ||
		tags[1].Name != "Морвен" || tags[1].PageCount != 2 {
		t.Fatalf("tags = %d %+v", rec.Code, tags)
	}
	city, morven := tags[0].ID, tags[1].ID

	rec = f.do(http.MethodPatch, "/api/wiki/tags/"+city, `{"name":"Город"}`, withCookie(f.gm))
	var renamed tagJSON
	decodeBody(t, rec, &renamed)
	if rec.Code != http.StatusOK || renamed.ID != city || renamed.Name != "Город" {
		t.Errorf("rename = %d %+v", rec.Code, renamed)
	}
	if rec := f.do(http.MethodPatch, "/api/wiki/tags/"+city, `{"name":"морвен"}`, withCookie(f.gm)); rec.Code != http.StatusConflict {
		t.Errorf("taken name = %d, want 409", rec.Code)
	}
	rec = f.do(http.MethodPatch, "/api/wiki/tags/"+city, `{"name":"морвен","merge":true}`, withCookie(f.gm))
	decodeBody(t, rec, &renamed)
	if rec.Code != http.StatusOK || renamed.ID != morven || renamed.Name != "Морвен" {
		t.Errorf("merge = %d %+v", rec.Code, renamed)
	}
	if rec := f.do(http.MethodPatch, "/api/wiki/tags/nope", `{"name":"x"}`, withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown tag = %d, want 404", rec.Code)
	}

	if rec := f.do(http.MethodDelete, "/api/wiki/tags/"+morven, "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	rec = f.do(http.MethodGet, camp+"/wiki/tags", "", withCookie(f.gm))
	tags = nil
	decodeBody(t, rec, &tags)
	if len(tags) != 0 {
		t.Errorf("tags after delete = %+v", tags)
	}
	if rec := f.do(http.MethodDelete, "/api/wiki/tags/"+morven, "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

func (f *wikiFixture) call(method, path, body string, want int, out any) {
	f.t.Helper()
	rec := f.do(method, path, body, withCookie(f.gm))
	if rec.Code != want {
		f.t.Fatalf("%s %s = %d %s, want %d", method, path, rec.Code, rec.Body.String(), want)
	}
	if out != nil {
		decodeBody(f.t, rec, out)
	}
}

func TestWikiDatabaseRequiresGM(t *testing.T) {
	f := newWikiFixture(t)
	db := f.createPage(`{"kind":"database","title":"DB","database":{"properties":[{"name":"N","type":"text"}]}}`)
	var data databaseJSON
	f.call(http.MethodGet, "/api/wiki/databases/"+db.ID, "", http.StatusOK, &data)
	rec := f.createPage(`{"parentId":"` + db.ID + `","title":"R"}`)
	_, link := f.startGame(f.gm, f.campaignID)
	player, _ := f.playerSession(link, "bob")

	routes := []struct{ method, path, body string }{
		{http.MethodPost, "/api/campaigns/" + f.campaignID + "/wiki/pages", `{"kind":"database"}`},
		{http.MethodGet, "/api/wiki/databases/" + db.ID, ""},
		{http.MethodPost, "/api/wiki/databases/" + db.ID + "/properties", `{"name":"X","type":"text"}`},
		{http.MethodPatch, "/api/wiki/properties/" + data.Properties[0].ID, `{"name":"Y"}`},
		{http.MethodDelete, "/api/wiki/properties/" + data.Properties[0].ID, ""},
		{http.MethodPost, "/api/wiki/databases/" + db.ID + "/views", `{"name":"V","kind":"table"}`},
		{http.MethodPatch, "/api/wiki/views/" + data.Views[0].ID, `{"name":"V"}`},
		{http.MethodDelete, "/api/wiki/views/" + data.Views[0].ID, ""},
		{http.MethodPatch, "/api/wiki/pages/" + rec.ID + "/props", `{"props":{}}`},
	}
	for _, rt := range routes {
		t.Run(rt.method+" "+rt.path, func(t *testing.T) {
			if rec := f.do(rt.method, rt.path, rt.body, withCookie(player)); rec.Code != http.StatusForbidden {
				t.Errorf("player = %d, want 403", rec.Code)
			}
			if rec := f.do(rt.method, rt.path, rt.body); rec.Code != http.StatusUnauthorized {
				t.Errorf("anonymous = %d, want 401", rec.Code)
			}
		})
	}
}

func TestWikiDatabaseLifecycle(t *testing.T) {
	f := newWikiFixture(t)

	db := f.createPage(`{"kind":"database","title":"NPC","database":{"properties":[
		{"name":"Status","type":"select","options":{"options":[{"name":"alive","color":"green"},{"name":"dead","color":"red"}]}}]}}`)
	if db.Kind != "database" {
		t.Fatalf("kind = %q", db.Kind)
	}
	var data databaseJSON
	f.call(http.MethodGet, "/api/wiki/databases/"+db.ID, "", http.StatusOK, &data)
	if len(data.Properties) != 1 || len(data.Views) != 1 || data.Views[0].Name != "Table" || data.Records == nil || len(data.Records) != 0 {
		t.Fatalf("database = %+v", data)
	}
	status := data.Properties[0]
	var opts struct {
		Options []struct{ ID, Name string }
	}
	if err := json.Unmarshal(status.Options, &opts); err != nil || len(opts.Options) != 2 || opts.Options[0].ID == "" {
		t.Fatalf("options = %s %v", status.Options, err)
	}
	alive, dead := opts.Options[0].ID, opts.Options[1].ID

	var prop propertyJSON
	f.call(http.MethodPost, "/api/wiki/databases/"+db.ID+"/properties", `{"name":"Age","type":"number","options":{"format":"integer"}}`, http.StatusCreated, &prop)
	if prop.Position != 1 || string(prop.Options) != `{"format":"integer"}` {
		t.Errorf("property = %+v", prop)
	}

	r1 := f.createPage(`{"parentId":"` + db.ID + `","title":"Barliman","props":{"` + status.ID + `":"` + alive + `"}}`)
	r2 := f.createPage(`{"parentId":"` + db.ID + `","title":"Nob"}`)
	if !r1.IsRecord || !strings.Contains(string(r1.Props), alive) {
		t.Errorf("record = %+v", r1)
	}
	if rec := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/wiki/pages",
		`{"parentId":"`+db.ID+`","props":{"`+status.ID+`":"made-up"}}`, withCookie(f.gm)); rec.Code != http.StatusBadRequest {
		t.Errorf("bad value = %d, want 400", rec.Code)
	}

	f.call(http.MethodGet, "/api/wiki/databases/"+db.ID, "", http.StatusOK, &data)
	if len(data.Records) != 2 || data.Records[0].ID != r1.ID || data.Records[0].Tags == nil ||
		!strings.Contains(string(data.Records[0].Props), alive) || data.Records[0].CreatedAt == 0 {
		t.Errorf("records = %+v", data.Records)
	}

	var tree struct {
		Pages []pageNodeJSON `json:"pages"`
	}
	f.call(http.MethodGet, "/api/campaigns/"+f.campaignID+"/wiki/tree", "", http.StatusOK, &tree)
	if len(tree.Pages) != 1 || tree.Pages[0].RecordCount != 2 {
		t.Errorf("tree = %+v", tree)
	}

	var set struct{ Props map[string]any }
	f.call(http.MethodPatch, "/api/wiki/pages/"+r2.ID+"/props",
		`{"props":{"`+status.ID+`":"`+dead+`","`+prop.ID+`":41}}`, http.StatusOK, &set)
	if set.Props[status.ID] != dead || set.Props[prop.ID] != float64(41) {
		t.Errorf("props = %+v", set.Props)
	}
	set.Props = nil
	f.call(http.MethodPatch, "/api/wiki/pages/"+r2.ID+"/props", `{"props":{"`+prop.ID+`":null}}`, http.StatusOK, &set)
	if _, has := set.Props[prop.ID]; has || set.Props[status.ID] != dead {
		t.Errorf("props after null = %+v", set.Props)
	}
	for name, tc := range map[string]struct{ path, body string }{
		"bad value":  {"/api/wiki/pages/" + r2.ID + "/props", `{"props":{"` + prop.ID + `":1.5}}`},
		"no props":   {"/api/wiki/pages/" + r2.ID + "/props", `{}`},
		"not record": {"/api/wiki/pages/" + db.ID + "/props", `{"props":{}}`},
		"extra key":  {"/api/wiki/pages/" + r2.ID + "/props", `{"props":{},"x":1}`},
	} {
		if rec := f.do(http.MethodPatch, tc.path, tc.body, withCookie(f.gm)); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
	if rec := f.do(http.MethodPatch, "/api/wiki/pages/nope/props", `{"props":{}}`, withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("missing record = %d, want 404", rec.Code)
	}

	var patched propertyJSON
	f.call(http.MethodPatch, "/api/wiki/properties/"+prop.ID, `{"name":"Years","position":0}`, http.StatusOK, &patched)
	if patched.Name != "Years" || patched.Position != 0 || patched.Type != "number" {
		t.Errorf("patched = %+v", patched)
	}
	if rec := f.do(http.MethodPatch, "/api/wiki/properties/"+prop.ID, `{"type":"text"}`, withCookie(f.gm)); rec.Code != http.StatusBadRequest {
		t.Errorf("type change = %d, want 400", rec.Code)
	}

	var view viewJSON
	f.call(http.MethodPost, "/api/wiki/databases/"+db.ID+"/views", `{"name":"Cards","kind":"cards","config":{"sorts":[{"field":"title","dir":"asc"}]}}`, http.StatusCreated, &view)
	if view.Position != 1 || !strings.Contains(string(view.Config), `"sorts":[{"field":"title","dir":"asc"}]`) {
		t.Errorf("view = %+v", view)
	}
	f.call(http.MethodPatch, "/api/wiki/views/"+view.ID, `{"name":"Gallery","position":0}`, http.StatusOK, &view)
	if view.Name != "Gallery" || view.Position != 0 {
		t.Errorf("patched view = %+v", view)
	}
	if rec := f.do(http.MethodPatch, "/api/wiki/views/"+view.ID, `{"config":{"zzz":1}}`, withCookie(f.gm)); rec.Code != http.StatusBadRequest {
		t.Errorf("bad config = %d, want 400", rec.Code)
	}

	// Deleting the property clears the values and the page shows it too.
	f.call(http.MethodDelete, "/api/wiki/properties/"+status.ID, "", http.StatusNoContent, nil)
	var page pageJSON
	f.call(http.MethodGet, "/api/wiki/pages/"+r1.ID, "", http.StatusOK, &page)
	if string(page.Props) != "{}" {
		t.Errorf("props after delete = %s", page.Props)
	}
	f.call(http.MethodDelete, "/api/wiki/properties/"+status.ID, "", http.StatusNotFound, nil)

	f.call(http.MethodGet, "/api/wiki/databases/"+db.ID, "", http.StatusOK, &data)
	if len(data.Properties) != 1 || data.Properties[0].ID != prop.ID || data.Properties[0].Position != 0 {
		t.Errorf("properties = %+v", data.Properties)
	}

	// The last view cannot go.
	f.call(http.MethodDelete, "/api/wiki/views/"+view.ID, "", http.StatusNoContent, nil)
	var left databaseJSON
	f.call(http.MethodGet, "/api/wiki/databases/"+db.ID, "", http.StatusOK, &left)
	f.call(http.MethodDelete, "/api/wiki/views/"+left.Views[0].ID, "", http.StatusConflict, nil)

	// A page is not a database.
	plain := f.createPage(`{"title":"Plain"}`)
	f.call(http.MethodGet, "/api/wiki/databases/"+plain.ID, "", http.StatusNotFound, nil)
	f.call(http.MethodPost, "/api/wiki/databases/"+plain.ID+"/properties", `{"name":"X","type":"text"}`, http.StatusNotFound, nil)
	for name, body := range map[string]string{
		"database with content": `{"kind":"database","content":` + emptyPageDoc + `}`,
		"props on a page":       `{"props":{"x":1}}`,
		"unknown kind":          `{"kind":"folder"}`,
		"database in database":  `{"kind":"database","parentId":"` + db.ID + `"}`,
	} {
		if rec := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/wiki/pages", body, withCookie(f.gm)); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", name, rec.Code)
		}
	}
}

func TestWikiDatabaseRelations(t *testing.T) {
	f := newWikiFixture(t)
	npc := f.createPage(`{"kind":"database","title":"NPC"}`)
	boss := f.createPage(`{"parentId":"` + npc.ID + `","title":"Boss"}`)
	quests := f.createPage(`{"kind":"database","title":"Quests","database":{"properties":[
		{"name":"Giver","type":"relation","options":{"targetDatabaseId":"` + npc.ID + `"}}]}}`)
	var data databaseJSON
	f.call(http.MethodGet, "/api/wiki/databases/"+quests.ID, "", http.StatusOK, &data)
	giver := data.Properties[0].ID
	quest := f.createPage(`{"parentId":"` + quests.ID + `","title":"Quest","props":{"` + giver + `":["` + boss.ID + `"]}}`)

	var page pageJSON
	f.call(http.MethodGet, "/api/wiki/pages/"+quest.ID, "", http.StatusOK, &page)
	if ref := page.Refs["page:"+boss.ID]; !ref.Exists || ref.Label != "Boss" {
		t.Errorf("refs = %+v", page.Refs)
	}
	var back []pageRefJSON
	f.call(http.MethodGet, "/api/wiki/pages/"+boss.ID+"/backlinks", "", http.StatusOK, &back)
	if len(back) != 1 || back[0].ID != quest.ID {
		t.Errorf("backlinks = %+v", back)
	}
	f.call(http.MethodPatch, "/api/wiki/pages/"+quest.ID+"/props", `{"props":{"`+giver+`":null}}`, http.StatusOK, nil)
	f.call(http.MethodGet, "/api/wiki/pages/"+boss.ID+"/backlinks", "", http.StatusOK, &back)
	if len(back) != 0 {
		t.Errorf("backlinks after null = %+v", back)
	}
}

func TestWikiTemplates(t *testing.T) {
	f := newWikiFixture(t)
	icon := "\U0001F37A"
	src := f.createPage(`{"title":"Tavern","icon":"` + icon + `"}`)
	content := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Menu"}]}]}`
	rec := f.do(http.MethodPut, "/api/wiki/pages/"+src.ID, `{"version":1,"title":"Tavern","icon":"`+icon+`","content":`+content+`}`, withCookie(f.gm))
	if rec.Code != http.StatusOK {
		t.Fatalf("save page = %d %s", rec.Code, rec.Body.String())
	}

	base := "/api/campaigns/" + f.campaignID + "/wiki/templates"
	rec = f.do(http.MethodPost, base, `{"fromPageId":"`+src.ID+`","name":"Inn"}`, withCookie(f.gm))
	var tpl templateJSON
	decodeBody(t, rec, &tpl)
	if rec.Code != http.StatusCreated || tpl.ID == "" || tpl.Name != "Inn" || tpl.Icon != icon || tpl.CreatedAt == 0 {
		t.Fatalf("save template = %d %+v", rec.Code, tpl)
	}

	var list []templateJSON
	rec = f.do(http.MethodGet, base, "", withCookie(f.gm))
	decodeBody(t, rec, &list)
	if rec.Code != http.StatusOK || len(list) != 1 || list[0] != tpl {
		t.Errorf("list = %d %+v", rec.Code, list)
	}

	page := f.createPage(`{"title":"New","templateId":"` + tpl.ID + `"}`)
	if page.Icon != icon || !strings.Contains(string(page.Content), "Menu") {
		t.Errorf("page from template = %+v", page)
	}

	rec = f.do(http.MethodPatch, "/api/wiki/templates/"+tpl.ID, `{"name":"Pub"}`, withCookie(f.gm))
	decodeBody(t, rec, &tpl)
	if rec.Code != http.StatusOK || tpl.Name != "Pub" {
		t.Errorf("rename = %d %+v", rec.Code, tpl)
	}

	errs := []struct {
		name, method, path, body string
		want                     int
	}{
		{"template with content", http.MethodPost, "/api/campaigns/" + f.campaignID + "/wiki/pages", `{"title":"X","templateId":"` + tpl.ID + `","content":` + emptyPageDoc + `}`, http.StatusBadRequest},
		{"unknown template", http.MethodPost, "/api/campaigns/" + f.campaignID + "/wiki/pages", `{"title":"X","templateId":"nope"}`, http.StatusNotFound},
		{"unknown source", http.MethodPost, base, `{"fromPageId":"nope","name":"X"}`, http.StatusNotFound},
		{"empty name", http.MethodPost, base, `{"fromPageId":"` + src.ID + `","name":""}`, http.StatusBadRequest},
		{"rename to empty", http.MethodPatch, "/api/wiki/templates/" + tpl.ID, `{"name":""}`, http.StatusBadRequest},
		{"rename unknown", http.MethodPatch, "/api/wiki/templates/nope", `{"name":"X"}`, http.StatusNotFound},
		{"delete unknown", http.MethodDelete, "/api/wiki/templates/nope", "", http.StatusNotFound},
		{"unknown campaign", http.MethodGet, "/api/campaigns/nope/wiki/templates", "", http.StatusNotFound},
	}
	for _, tt := range errs {
		if rec := f.do(tt.method, tt.path, tt.body, withCookie(f.gm)); rec.Code != tt.want {
			t.Errorf("%s = %d %s, want %d", tt.name, rec.Code, rec.Body.String(), tt.want)
		}
	}

	if rec := f.do(http.MethodDelete, "/api/wiki/templates/"+tpl.ID, "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Errorf("delete = %d", rec.Code)
	}
	rec = f.do(http.MethodGet, base, "", withCookie(f.gm))
	list = nil
	decodeBody(t, rec, &list)
	if len(list) != 0 || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("list after delete = %s", rec.Body.String())
	}
}

func TestWikiExportPageHeaders(t *testing.T) {
	f := newWikiFixture(t)
	tests := []struct{ title, disposition string }{
		{"Таверна", `attachment; filename="page.md"; filename*=UTF-8''%D0%A2%D0%B0%D0%B2%D0%B5%D1%80%D0%BD%D0%B0.md`},
		{"A b (c)'*", `attachment; filename="page.md"; filename*=UTF-8''A%20b%20%28c%29%27.md`},
		{"🍺", `attachment; filename="page.md"; filename*=UTF-8''%F0%9F%8D%BA.md`},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			p := f.createPage(jsonBody(t, map[string]string{"title": tt.title}))
			rec := f.do(http.MethodGet, "/api/wiki/pages/"+p.ID+"/export.md", "", withCookie(f.gm))
			if rec.Code != http.StatusOK {
				t.Fatalf("export = %d %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Type"); got != "text/markdown; charset=utf-8" {
				t.Errorf("Content-Type = %q", got)
			}
			if got := rec.Header().Get("Content-Disposition"); got != tt.disposition {
				t.Errorf("Content-Disposition = %q, want %q", got, tt.disposition)
			}
			if !strings.HasPrefix(rec.Body.String(), "---\ntitle: ") {
				t.Errorf("body = %q", rec.Body.String())
			}
		})
	}
	if rec := f.do(http.MethodGet, "/api/wiki/pages/nope/export.md", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown page = %d, want 404", rec.Code)
	}
}

func TestWikiExportZip(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	img := f.mustUpload("map.png", testPNG(t, 2, 2))
	sum := sha256.Sum256(testPNG(t, 2, 2))
	sha := hex.EncodeToString(sum[:])

	create := func(body string) pageJSON {
		rec := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/wiki/pages", body, withCookie(f.gm))
		var p pageJSON
		decodeBody(t, rec, &p)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create page = %d %s", rec.Code, rec.Body.String())
		}
		return p
	}
	parent := create(`{"title":"Мир"}`)
	child := create(`{"title":"Город","parentId":"` + parent.ID + `"}`)
	content := `{"type":"doc","content":[{"type":"image","attrs":{"assetId":"` + img.ID + `","alt":"map"}}]}`
	if rec := f.do(http.MethodPut, "/api/wiki/pages/"+child.ID, `{"version":1,"title":"Город","content":`+content+`}`, withCookie(f.gm)); rec.Code != http.StatusOK {
		t.Fatalf("save page = %d %s", rec.Code, rec.Body.String())
	}

	rec := f.do(http.MethodGet, "/api/campaigns/"+f.campaignID+"/wiki/export.zip", "", withCookie(f.gm))
	if rec.Code != http.StatusOK {
		t.Fatalf("export = %d %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/zip" {
		t.Errorf("Content-Type = %q", got)
	}
	wantDisposition := `attachment; filename="wiki.zip"; filename*=UTF-8''wiki-Camp-2026-01-01.zip`
	if got := rec.Header().Get("Content-Disposition"); got != wantDisposition {
		t.Errorf("Content-Disposition = %q, want %q", got, wantDisposition)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, zf := range zr.File {
		names = append(names, zf.Name)
	}
	slices.Sort(names)
	want := []string{"assets/" + sha + ".png", "Мир.md", "Мир/Город.md"}
	slices.Sort(want)
	if !slices.Equal(names, want) {
		t.Errorf("archive = %q, want %q", names, want)
	}

	if rec := f.do(http.MethodGet, "/api/campaigns/nope/wiki/export.zip", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown campaign = %d, want 404", rec.Code)
	}
}

func TestWikiExportZipNameOfACampaignWithCyrillic(t *testing.T) {
	f := newWikiFixture(t)
	cid := f.createCampaign(f.gm, "Забытые: земли")
	rec := f.do(http.MethodGet, "/api/campaigns/"+cid+"/wiki/export.zip", "", withCookie(f.gm))
	if rec.Code != http.StatusOK {
		t.Fatalf("export = %d", rec.Code)
	}
	want := `attachment; filename="wiki.zip"; filename*=UTF-8''wiki-%D0%97%D0%B0%D0%B1%D1%8B%D1%82%D1%8B%D0%B5%20%D0%B7%D0%B5%D0%BC%D0%BB%D0%B8-2026-01-01.zip`
	if got := rec.Header().Get("Content-Disposition"); got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
}
