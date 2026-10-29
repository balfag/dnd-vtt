package httpapi

import (
	"net/http"
	"strings"
	"testing"
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
		{"post with kind", http.MethodPost, pages, `{"kind":"database"}`, http.StatusBadRequest},
		{"post with props", http.MethodPost, pages, `{"props":{}}`, http.StatusBadRequest},
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
