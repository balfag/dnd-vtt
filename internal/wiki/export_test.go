package wiki

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestSafeFileName(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "Таверна", "Таверна"},
		{"forbidden characters", `a\b/c:d*e?f"g<h>i|j`, "abcdefghij"},
		{"control characters", "a\x00b\nc\td", "abcd"},
		{"edge spaces", "  spaced  ", "spaced"},
		{"trailing dots", "Foo...", "Foo"},
		{"empty", "", "Без названия"},
		{"only forbidden", `/\:`, "Без названия"},
		{"only spaces", "   ", "Без названия"},
		{"emoji stay", "🍺 Таверна", "🍺 Таверна"},
		{"100 code points", strings.Repeat("я", 100), strings.Repeat("я", 100)},
		{"101 code points", strings.Repeat("я", 101), strings.Repeat("я", 100)},
		{"cut then trimmed", strings.Repeat("a", 99) + " b", strings.Repeat("a", 99)},
		{"invalid UTF-8", "a\xffb", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SafeFileName(tt.in); got != tt.want {
				t.Errorf("SafeFileName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestLayoutNamesAreUniquePerFolder(t *testing.T) {
	child := &exPage{title: "Foo"}
	roots := []*exPage{
		{title: "Foo", children: []*exPage{child}},
		{title: "foo"},
		{title: "FOO"},
		{title: "Foo (2)"},
		{title: ""},
		{title: "?"},
	}
	layout(roots, "")
	var got []string
	for _, p := range roots {
		got = append(got, p.path)
	}
	// "Foo (2)" collides with the generated "foo (2)": it gets the next number.
	want := []string{"Foo.md", "foo (2).md", "FOO (3).md", "Foo (2) (2).md", "Без названия.md", "Без названия (2).md"}
	if !slices.Equal(got, want) {
		t.Errorf("paths = %q, want %q", got, want)
	}
	if child.path != "Foo/Foo.md" {
		t.Errorf("child path = %q", child.path)
	}
}

func TestRelPath(t *testing.T) {
	tests := []struct{ from, to, want string }{
		{"", "A.md", "A.md"},
		{"", "A/B.md", "A/B.md"},
		{"A/", "A/B.md", "B.md"},
		{"A/", "C.md", "../C.md"},
		{"A/B/", "A/C/D.md", "../C/D.md"},
		{"A/B/", "NPC/Мирта Гроссвальд.md", "../../NPC/Мирта%20Гроссвальд.md"},
		{"A/", "A/B (1).md", "B%20%281%29.md"},
	}
	for _, tt := range tests {
		if got := relPath(tt.from, tt.to); got != tt.want {
			t.Errorf("relPath(%q, %q) = %q, want %q", tt.from, tt.to, got, tt.want)
		}
	}
}

func TestYAMLString(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Таверна «Сонный грифон»", `"Таверна «Сонный грифон»"`},
		{"🍺", `"🍺"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{"two\nlines\ttab", `"two\nlines\ttab"`},
		{"\x01", `"\x01"`},
	}
	for _, tt := range tests {
		if got := yamlString(tt.in); got != tt.want {
			t.Errorf("yamlString(%q) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

// exportEnv is a campaign with databases and a page tree to export.
type exportEnv struct {
	*env
	npcDB, questDB       string
	mirta, quest         string
	world, cities, town  string
	tavern, trashed      string
	assetID, sha         string
	files                *fakeAssets
	townText, tavernText string
}

type fakeAssets struct {
	e     *env
	opens map[string]int
}

func (f *fakeAssets) Open(_ context.Context, id string) (io.ReadCloser, string, string, error) {
	var sha string
	f.e.queryRow(&sha, `SELECT sha256 FROM asset WHERE id = ?`, id)
	f.opens[id]++
	return io.NopCloser(strings.NewReader("PNG:" + id)), "png", sha, nil
}

func (e *env) saveContent(pageID, content string) {
	e.t.Helper()
	p, err := e.svc.Get(context.Background(), pageID)
	if err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.svc.Save(context.Background(), pageID, SavePage{Version: p.Version, Title: p.Title, Icon: p.Icon, Content: json.RawMessage(content)}); err != nil {
		e.t.Fatal(err)
	}
}

func optionID(p Property, name string) string {
	var so selectOptions
	if err := json.Unmarshal(p.Options, &so); err != nil {
		panic(err)
	}
	for _, o := range so.Options {
		if o.Name == name {
			return o.ID
		}
	}
	panic("no option " + name)
}

func newExportEnv(t *testing.T) *exportEnv {
	t.Helper()
	e := &exportEnv{env: newEnv(t)}
	ctx := context.Background()
	e.files = &fakeAssets{e: e.env, opens: map[string]int{}}
	e.assetID = e.asset(e.camp)
	e.queryRow(&e.sha, `SELECT sha256 FROM asset WHERE id = ?`, e.assetID)

	var npcProps map[string]Property
	e.npcDB, npcProps = e.newDB("NPC",
		NewProperty{Name: "Статус", Type: PropSelect, Options: json.RawMessage(`{"options":[{"name":"жив","color":"green"},{"name":"мёртв","color":"red"}]}`)},
		NewProperty{Name: "Роли", Type: PropMultiSelect, Options: json.RawMessage(`{"options":[{"name":"торговец","color":"blue"},{"name":"шпион","color":"gray"}]}`)},
		NewProperty{Name: "Уровень", Type: PropNumber},
		NewProperty{Name: "Союзник", Type: PropCheckbox},
		NewProperty{Name: "Заметка", Type: PropText},
	)
	rec, err := e.svc.Create(ctx, e.camp, NewPage{ParentID: e.npcDB, Title: "Мирта Гроссвальд"})
	if err != nil {
		t.Fatal(err)
	}
	e.mirta = rec.ID
	mustProps(t, e.env, e.mirta, map[string]string{
		npcProps["Статус"].ID:  `"` + optionID(npcProps["Статус"], "жив") + `"`,
		npcProps["Роли"].ID:    `["` + optionID(npcProps["Роли"], "торговец") + `","` + optionID(npcProps["Роли"], "шпион") + `"]`,
		npcProps["Уровень"].ID: `3`,
		npcProps["Союзник"].ID: `true`,
	})
	if _, err := e.svc.SetTags(ctx, e.mirta, []string{"Морвен", "гильдия"}); err != nil {
		t.Fatal(err)
	}
	scene := e.scene(e.camp, "Погреб")
	e.token(scene, "Грифон")

	var questProps map[string]Property
	e.questDB, questProps = e.newDB("Квесты",
		NewProperty{Name: "Связанные NPC", Type: PropRelation, Options: json.RawMessage(`{"targetDatabaseId":"` + e.npcDB + `"}`)},
		NewProperty{Name: "Сцены", Type: PropGameLink},
	)
	q, err := e.svc.Create(ctx, e.camp, NewPage{ParentID: e.questDB, Title: "Найти | Мирту"})
	if err != nil {
		t.Fatal(err)
	}
	e.quest = q.ID
	var sceneID string
	e.queryRow(&sceneID, `SELECT id FROM scene WHERE campaign_id = ?`, e.camp)
	mustProps(t, e.env, e.quest, map[string]string{
		questProps["Связанные NPC"].ID: `["` + e.mirta + `"]`,
		questProps["Сцены"].ID:         `[{"kind":"scene","id":"` + sceneID + `"}]`,
	})
	if _, err := e.svc.SetTags(ctx, e.quest, []string{"срочно"}); err != nil {
		t.Fatal(err)
	}

	e.world = e.create("", "Мир").ID
	e.cities = e.create(e.world, "Города").ID
	e.town = e.create(e.cities, "Морвен").ID
	e.tavern = e.create(e.world, "Таверна «Сонный грифон»").ID
	e.trashed = e.create("", "Старое").ID

	e.townText = `{"type":"doc","content":[
		{"type":"paragraph","content":[` + text("Лавка ") + `,{"type":"mention","attrs":{"kind":"page","id":"` + e.mirta + `","label":"Старое имя"}}]},
		` + image(e.assetID) + `]}`
	e.saveContent(e.town, e.townText)
	e.tavernText = `{"type":"doc","content":[
		{"type":"paragraph","content":[` + text("Рядом ") + `,{"type":"mention","attrs":{"kind":"page","id":"` + e.town + `"}},` + text(" и ") + `,{"type":"mention","attrs":{"kind":"page","id":"` + e.trashed + `","label":"Старое"}}]},
		` + image(e.assetID) + `]}`
	e.saveContent(e.tavern, e.tavernText)
	if err := e.svc.Delete(ctx, e.trashed); err != nil {
		t.Fatal(err)
	}
	return e
}

func mustProps(t *testing.T, e *env, recordID string, values map[string]string) {
	t.Helper()
	if _, err := e.setProps(recordID, values); err != nil {
		t.Fatal(err)
	}
}

func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		out[f.Name] = string(b)
	}
	return out
}

func TestExportPageFrontMatterOfRecords(t *testing.T) {
	e := newExportEnv(t)
	ctx := context.Background()

	name, md, err := e.svc.ExportPage(ctx, e.mirta)
	if err != nil {
		t.Fatal(err)
	}
	want := `---
title: "Мирта Гроссвальд"
icon: ""
tags: ["Морвен", "гильдия"]
props:
  "Статус": "жив"
  "Роли": ["торговец", "шпион"]
  "Уровень": 3
  "Союзник": true
---

# Мирта Гроссвальд
`
	if name != "Мирта Гроссвальд.md" || string(md) != want {
		t.Errorf("ExportPage() = %q\n%s\nwant\n%s", name, md, want)
	}

	name, md, err = e.svc.ExportPage(ctx, e.quest)
	if err != nil {
		t.Fatal(err)
	}
	want = `---
title: "Найти | Мирту"
icon: ""
tags: ["срочно"]
props:
  "Связанные NPC": ["Мирта Гроссвальд"]
  "Сцены": ["Погреб"]
---

# Найти | Мирту
`
	if name != "Найти  Мирту.md" || string(md) != want {
		t.Errorf("ExportPage() = %q\n%s\nwant\n%s", name, md, want)
	}
}

func TestExportPageQuotesAndEmoji(t *testing.T) {
	e := newEnv(t)
	p, err := e.svc.Create(context.Background(), e.camp, NewPage{Title: `Говорит "Привет" \o/`, Icon: "\U0001F37A"})
	if err != nil {
		t.Fatal(err)
	}
	_, md, err := e.svc.ExportPage(context.Background(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ntitle: \"Говорит \\\"Привет\\\" \\\\o/\"\nicon: \"\U0001F37A\"\ntags: []\n---\n\n# Говорит \"Привет\" \\\\o/\n"
	if string(md) != want {
		t.Errorf("ExportPage() =\n%s\nwant\n%s", md, want)
	}
}

func TestExportPageRendersBodyWithoutLinks(t *testing.T) {
	e := newExportEnv(t)
	name, md, err := e.svc.ExportPage(context.Background(), e.tavern)
	if err != nil {
		t.Fatal(err)
	}
	body := string(md)
	for _, want := range []string{
		"Рядом Морвен и Старое\n",
		"![](/assets/" + e.assetID + ")\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("%s lacks %q:\n%s", name, want, body)
		}
	}
	if strings.Contains(body, "](") && strings.Contains(body, ".md") {
		t.Errorf("a link to a page in a single export:\n%s", body)
	}
}

func TestExportPageOfDatabaseHasTheTable(t *testing.T) {
	e := newExportEnv(t)
	_, md, err := e.svc.ExportPage(context.Background(), e.npcDB)
	if err != nil {
		t.Fatal(err)
	}
	want := "# NPC\n\n| Название | Статус | Роли | Уровень | Союзник | Заметка |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| Мирта Гроссвальд | жив | торговец, шпион | 3 | true |  |\n"
	if !strings.HasSuffix(string(md), want) {
		t.Errorf("database export:\n%s\nwant suffix\n%s", md, want)
	}
}

func TestExportPageNotFound(t *testing.T) {
	e := newExportEnv(t)
	for _, id := range []string{e.trashed, "nope"} {
		if _, _, err := e.svc.ExportPage(context.Background(), id); !errors.Is(err, ErrNotFound) {
			t.Errorf("ExportPage(%q) error = %v, want ErrNotFound", id, err)
		}
	}
}

func TestExportAll(t *testing.T) {
	e := newExportEnv(t)
	var buf bytes.Buffer
	if err := e.svc.ExportAll(context.Background(), e.camp, &buf, e.files); err != nil {
		t.Fatal(err)
	}
	files := readZip(t, buf.Bytes())

	wantNames := []string{
		"NPC.md", "NPC/Мирта Гроссвальд.md",
		"Квесты.md", "Квесты/Найти  Мирту.md",
		"Мир.md", "Мир/Города.md", "Мир/Города/Морвен.md", "Мир/Таверна «Сонный грифон».md",
		"assets/" + e.sha + ".png",
	}
	slices.Sort(wantNames)
	if got := slices.Sorted(maps.Keys(files)); !slices.Equal(got, wantNames) {
		t.Fatalf("archive has\n%q\nwant\n%q", got, wantNames)
	}

	// Pages in the trash are not exported, neither is their name in a link.
	for name, body := range files {
		if strings.Contains(name, "Старое") || (strings.HasSuffix(name, ".md") && strings.Contains(body, "Старое.md")) {
			t.Errorf("trash leaked into %s", name)
		}
	}

	// The image is stored once, though two pages use it.
	if e.files.opens[e.assetID] != 1 || files["assets/"+e.sha+".png"] != "PNG:"+e.assetID {
		t.Errorf("asset opened %d times, content %q", e.files.opens[e.assetID], files["assets/"+e.sha+".png"])
	}

	// Relative links between pages in different folders and to the image.
	town := files["Мир/Города/Морвен.md"]
	for _, want := range []string{
		"title: \"Морвен\"\n",
		"Лавка [Мирта Гроссвальд](../../NPC/Мирта%20Гроссвальд.md)\n",
		"![](../../assets/" + e.sha + ".png)\n",
	} {
		if !strings.Contains(town, want) {
			t.Errorf("Морвен.md lacks %q:\n%s", want, town)
		}
	}
	tavern := files["Мир/Таверна «Сонный грифон».md"]
	for _, want := range []string{
		"Рядом [Морвен](Города/Морвен.md) и Старое\n",
		"![](../assets/" + e.sha + ".png)\n",
	} {
		if !strings.Contains(tavern, want) {
			t.Errorf("Таверна.md lacks %q:\n%s", want, tavern)
		}
	}

	// A database: front matter and the table of its records.
	npc := files["NPC.md"]
	wantTable := "# NPC\n\n| Название | Статус | Роли | Уровень | Союзник | Заметка |\n| --- | --- | --- | --- | --- | --- |\n" +
		"| [Мирта Гроссвальд](NPC/Мирта%20Гроссвальд.md) | жив | торговец, шпион | 3 | true |  |\n"
	if !strings.HasPrefix(npc, "---\ntitle: \"NPC\"\n") || !strings.HasSuffix(npc, wantTable) {
		t.Errorf("NPC.md:\n%s", npc)
	}
	quests := files["Квесты.md"]
	if !strings.Contains(quests, "| [Найти \\| Мирту](Квесты/Найти%20%20Мирту.md) | Мирта Гроссвальд | Погреб |\n") {
		t.Errorf("Квесты.md:\n%s", quests)
	}

	// The record keeps its values in the front matter.
	if rec := files["NPC/Мирта Гроссвальд.md"]; !strings.Contains(rec, "props:\n  \"Статус\": \"жив\"\n") || !strings.HasPrefix(rec, "---\n") {
		t.Errorf("record:\n%s", rec)
	}
}

func TestExportAllUsesTheFirstViewColumns(t *testing.T) {
	e := newExportEnv(t)
	ctx := context.Background()
	db, err := e.svc.Database(ctx, e.npcDB)
	if err != nil {
		t.Fatal(err)
	}
	props := e.props(e.npcDB)
	visible := `["` + props["Уровень"].ID + `","` + props["Статус"].ID + `"]`
	if _, err := e.svc.UpdateView(ctx, db.Views[0].ID, ViewPatch{Config: json.RawMessage(`{"visible":` + visible + `}`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.AddView(ctx, e.npcDB, "Cards", ViewCards, nil); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := e.svc.ExportAll(ctx, e.camp, &buf, e.files); err != nil {
		t.Fatal(err)
	}
	npc := readZip(t, buf.Bytes())["NPC.md"]
	if !strings.Contains(npc, "| Название | Уровень | Статус |\n| --- | --- | --- |\n| [Мирта Гроссвальд](NPC/Мирта%20Гроссвальд.md) | 3 | жив |\n") {
		t.Errorf("NPC.md:\n%s", npc)
	}
}

func TestExportAllErrors(t *testing.T) {
	e := newExportEnv(t)
	var buf bytes.Buffer
	if err := e.svc.ExportAll(context.Background(), "nope", &buf, e.files); !errors.Is(err, ErrNotFound) || buf.Len() != 0 {
		t.Errorf("unknown campaign: %v, %d bytes written", err, buf.Len())
	}
	if err := e.svc.ExportAll(context.Background(), e.camp, &buf, nil); err == nil {
		t.Error("images without asset files: want an error")
	}
}

func TestExportAllOfAnEmptyCampaign(t *testing.T) {
	e := newEnv(t)
	var buf bytes.Buffer
	if err := e.svc.ExportAll(context.Background(), e.camp, &buf, nil); err != nil {
		t.Fatal(err)
	}
	if files := readZip(t, buf.Bytes()); len(files) != 0 {
		t.Errorf("files = %v", files)
	}
}
