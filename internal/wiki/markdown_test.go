package wiki

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/wiki/doc"
)

var update = flag.Bool("update", false, "rewrite the golden .md files")

// goldenLinker knows the page 01ARZ3NDEKTSV4RRFFQ69G5FAV (renamed since the
// mention was written) and not the page ...FAW.
type goldenLinker struct{}

func (goldenLinker) Page(id string) (string, bool) {
	if id == "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		return "../NPC/New%20name.md", true
	}
	return "", false
}

func (goldenLinker) Asset(id string) string { return "/assets/" + id }

func (goldenLinker) Label(r doc.Ref, stored string) string {
	if r.ID == "01ARZ3NDEKTSV4RRFFQ69G5FAV" {
		return "New name"
	}
	return stored
}

func normalize(t *testing.T, raw []byte) []byte {
	t.Helper()
	res, err := doc.Validate(raw, doc.Options{AssetOK: func(string) (bool, error) { return true, nil }})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	return res.JSON
}

func TestToMarkdownGolden(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "markdown", "*.json"))
	if err != nil || len(inputs) == 0 {
		t.Fatalf("no golden inputs: %v", err)
	}
	for _, in := range inputs {
		name := strings.TrimSuffix(filepath.Base(in), ".json")
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(in)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ToMarkdown(normalize(t, raw), goldenLinker{})
			if err != nil {
				t.Fatal(err)
			}
			got += "\n"
			golden := strings.TrimSuffix(in, ".json") + ".md"
			if *update {
				if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
				t.Errorf("markdown differs from %s:\n%s", golden, got)
			}
		})
	}
}

func TestToMarkdownEmptyDocument(t *testing.T) {
	got, err := ToMarkdown(doc.Empty(), goldenLinker{})
	if err != nil || got != "" {
		t.Errorf("ToMarkdown(empty) = %q, %v", got, err)
	}
}

func TestToMarkdownRejectsBadInput(t *testing.T) {
	for name, raw := range map[string]string{
		"not json":     `{`,
		"not a doc":    `{"type":"paragraph"}`,
		"unknown node": `{"type":"doc","content":[{"type":"video"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ToMarkdown([]byte(raw), goldenLinker{}); err == nil {
				t.Error("want an error")
			}
		})
	}
}
