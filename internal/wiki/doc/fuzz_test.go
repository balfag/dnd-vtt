package doc

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func FuzzValidate(f *testing.F) {
	full, err := os.ReadFile(filepath.Join("testdata", "full.json"))
	if err != nil {
		f.Fatal(err)
	}
	f.Add(full, false)
	f.Add(full, true)
	f.Add(Empty(), false)
	for _, s := range []string{
		`{"type":"doc"}`,
		`{"type":"doc","content":[{"type":"heading"}]}`,
		`{"type":"doc","content":[{"type":"heading","attrs":{"level":1e400}}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":""}]}]}`,
		`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colwidth":[39]},"content":[{"type":"paragraph"}]}]}]}]}`,
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]}]}]}`,
		`{"type":"doc","content":[{"type":"details","content":[{"type":"detailsSummary"}]}]}`,
		`[[[[[[[[`,
	} {
		f.Add([]byte(s), false)
	}

	f.Fuzz(func(t *testing.T, raw []byte, handout bool) {
		o := Options{Handout: handout, AssetOK: anyAsset}
		res, err := Validate(raw, o)
		if err != nil {
			return
		}
		again, err := Validate(res.JSON, o)
		if err != nil {
			t.Fatalf("normalized document is invalid: %v\n%s", err, res.JSON)
		}
		if !bytes.Equal(again.JSON, res.JSON) {
			t.Errorf("JSON is not idempotent:\n got: %s\nwant: %s", again.JSON, res.JSON)
		}
		if again.Text != res.Text {
			t.Errorf("Text = %q, want %q", again.Text, res.Text)
		}
		if !slices.Equal(again.Mentions, res.Mentions) {
			t.Errorf("Mentions = %v, want %v", again.Mentions, res.Mentions)
		}
		if !slices.Equal(again.Assets, res.Assets) {
			t.Errorf("Assets = %v, want %v", again.Assets, res.Assets)
		}
	})
}
