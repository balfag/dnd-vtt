package httpapi

import (
	"encoding/json"
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
