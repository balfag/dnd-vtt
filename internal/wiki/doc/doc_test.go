package doc

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files in testdata")

const (
	ulid1 = "01HZX3K9M2N4P6Q8R0S1T2V3W4"
	ulid2 = "01HZX3K9M2N4P6Q8R0S1T2V3W5"
)

func anyAsset(string) (bool, error) { return true, nil }

func opts() Options { return Options{AssetOK: anyAsset} }

// wrap puts blocks (comma-separated JSON) into a document.
func wrap(blocks string) []byte {
	return []byte(`{"type":"doc","content":[` + blocks + `]}`)
}

// para wraps inline nodes into a paragraph.
func para(inline ...string) string {
	return `{"type":"paragraph","content":[` + strings.Join(inline, ",") + `]}`
}

func txt(s string) string { return `{"type":"text","text":"` + s + `"}` }

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	root, err := os.OpenRoot("testdata")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	b, err := root.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestValidateFullGolden(t *testing.T) {
	res, err := Validate(readTestdata(t, "full.json"), opts())
	if err != nil {
		t.Fatal(err)
	}
	if *update {
		root, err := os.OpenRoot("testdata")
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = root.Close() }()
		if err := root.WriteFile("full.golden.json", append(bytes.Clone(res.JSON), '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want := bytes.TrimSpace(readTestdata(t, "full.golden.json"))
	if !bytes.Equal(res.JSON, want) {
		t.Errorf("normalized JSON differs from testdata/full.golden.json.\n got: %s\nwant: %s\nRun `go test ./internal/wiki/doc/ -update` if the change is intended.", res.JSON, want)
	}

	again, err := Validate(res.JSON, opts())
	if err != nil {
		t.Fatalf("revalidate: %v", err)
	}
	if !bytes.Equal(again.JSON, res.JSON) {
		t.Errorf("not idempotent:\n got: %s\nwant: %s", again.JSON, res.JSON)
	}
	for _, extra := range []string{"textAlign", "src", "title", "target", "rel", "class", "backgroundColor", "extra", "foo"} {
		if bytes.Contains(res.JSON, []byte(extra)) {
			t.Errorf("extra attribute %q was kept", extra)
		}
	}
}

func TestValidateInvalid(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		path string // substring expected in the error
	}{
		{"unknown node", wrapS(`{"type":"video"}`), "content[0].type"},
		{"unknown mark", wrapS(para(`{"type":"text","text":"a","marks":[{"type":"sparkle"}]}`)), "content[0].content[0].marks[0].type"},
		{"missing type", wrapS(`{"content":[]}`), "content[0].type"},
		{"node not an object", wrapS(`5`), "content[0]"},
		{"root is not doc", `{"type":"paragraph"}`, "type: node"},
		{"root is not an object", `[]`, "<root>"},
		{"doc nested", wrapS(`{"type":"doc","content":[]}`), "content[0].type"},
		{"empty doc content", `{"type":"doc","content":[]}`, "content"},
		{"doc without content", `{"type":"doc"}`, "content"},
		{"content not an array", `{"type":"doc","content":{}}`, "content"},
		{"attrs not an object", wrapS(`{"type":"heading","attrs":5}`), "content[0].attrs"},
		{"mark not an object", wrapS(para(`{"type":"text","text":"a","marks":[1]}`)), "marks[0]"},
		{"mark without type", wrapS(para(`{"type":"text","text":"a","marks":[{}]}`)), "marks[0].type"},
		{"marks not an array", wrapS(para(`{"type":"text","text":"a","marks":{}}`)), "marks"},

		{"heading without level", wrapS(`{"type":"heading"}`), "content[0].attrs.level"},
		{"heading null level", wrapS(`{"type":"heading","attrs":{"level":null}}`), "attrs.level"},
		{"heading level 0", wrapS(`{"type":"heading","attrs":{"level":0}}`), "attrs.level"},
		{"heading level 4", wrapS(`{"type":"heading","attrs":{"level":4}}`), "attrs.level"},
		{"heading level 2.5", wrapS(`{"type":"heading","attrs":{"level":2.5}}`), "attrs.level"},
		{"heading level string", wrapS(`{"type":"heading","attrs":{"level":"2"}}`), "attrs.level"},
		{"heading level overflow", wrapS(`{"type":"heading","attrs":{"level":1e400}}`), "attrs.level"},
		{"heading level huge", wrapS(`{"type":"heading","attrs":{"level":1e30}}`), "attrs.level"},
		{"orderedList start 0", wrapS(`{"type":"orderedList","attrs":{"start":0},"content":[` + li() + `]}`), "attrs.start"},
		{"orderedList start 10001", wrapS(`{"type":"orderedList","attrs":{"start":10001},"content":[` + li() + `]}`), "attrs.start"},
		{"taskItem checked string", wrapS(`{"type":"taskList","content":[{"type":"taskItem","attrs":{"checked":"yes"},"content":[` + para("") + `]}]}`), "attrs.checked"},
		{"details open number", wrapS(`{"type":"details","attrs":{"open":1},"content":[` + detailsBody() + `]}`), "attrs.open"},

		{"listItem not starting with paragraph", wrapS(`{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"horizontalRule"}]}]}`), "content[0].content[0].content[0].type"},
		{"empty listItem", wrapS(`{"type":"bulletList","content":[{"type":"listItem"}]}`), "content[0].content[0].content"},
		{"empty bulletList", wrapS(`{"type":"bulletList","content":[]}`), "content[0].content"},
		{"bulletList with paragraph", wrapS(`{"type":"bulletList","content":[` + para("") + `]}`), "content[0].content[0].type"},
		{"empty orderedList", wrapS(`{"type":"orderedList"}`), "content[0].content"},
		{"empty taskList", wrapS(`{"type":"taskList"}`), "content[0].content"},
		{"empty taskItem", wrapS(`{"type":"taskList","content":[{"type":"taskItem"}]}`), "content[0].content[0].content"},
		{"empty blockquote", wrapS(`{"type":"blockquote"}`), "content[0].content"},
		{"empty callout", wrapS(`{"type":"callout"}`), "content[0].content"},
		{"text with empty string", wrapS(para(txt(""))), "content[0].content[0].text"},
		{"text without text", wrapS(para(`{"type":"text"}`)), "content[0].content[0].text"},
		{"text in doc", wrapS(txt("a")), "content[0].type"},
		{"paragraph in paragraph", wrapS(para(para(""))), "content[0].content[0].type"},
		{"heading in paragraph", wrapS(para(`{"type":"heading","attrs":{"level":1}}`)), "content[0].content[0].type"},
		{"listItem in doc", wrapS(`{"type":"listItem","content":[` + para("") + `]}`), "content[0].type"},
		{"tableRow in doc", wrapS(`{"type":"tableRow"}`), "content[0].type"},
		{"leaf with content", wrapS(`{"type":"horizontalRule","content":[` + para("") + `]}`), "content[0].content"},
		{"hardBreak with content", wrapS(para(`{"type":"hardBreak","content":[` + txt("a") + `]}`)), "content[0].content[0].content"},
		{"image with content", wrapS(`{"type":"image","attrs":{"assetId":"A"},"content":[` + para("") + `]}`), "content[0].content"},

		{"callout empty emoji", wrapS(`{"type":"callout","attrs":{"emoji":""},"content":[` + para("") + `]}`), "attrs.emoji"},
		{"callout emoji too long", wrapS(`{"type":"callout","attrs":{"emoji":"123456789"},"content":[` + para("") + `]}`), "attrs.emoji"},
		{"callout emoji control", wrapS(`{"type":"callout","attrs":{"emoji":"\n"},"content":[` + para("") + `]}`), "attrs.emoji"},
		{"callout emoji number", wrapS(`{"type":"callout","attrs":{"emoji":5},"content":[` + para("") + `]}`), "attrs.emoji"},

		{"codeBlock marked text", wrapS(`{"type":"codeBlock","content":[{"type":"text","text":"a","marks":[{"type":"bold"}]}]}`), "content[0].content[0].marks"},
		{"codeBlock with mention", wrapS(`{"type":"codeBlock","content":[{"type":"mention","attrs":{"kind":"page","id":"` + ulid1 + `"}}]}`), "content[0].content[0].type"},
		{"codeBlock uppercase language", wrapS(`{"type":"codeBlock","attrs":{"language":"C"}}`), "attrs.language"},
		{"codeBlock 21-char language", wrapS(`{"type":"codeBlock","attrs":{"language":"` + strings.Repeat("a", 21) + `"}}`), "attrs.language"},
		{"codeBlock space in language", wrapS(`{"type":"codeBlock","attrs":{"language":"a b"}}`), "attrs.language"},

		{"table without rows", wrapS(`{"type":"table"}`), "content[0].content"},
		{"tableRow without cells", wrapS(`{"type":"table","content":[{"type":"tableRow"}]}`), "content[0].content[0].content"},
		{"table with paragraph", wrapS(`{"type":"table","content":[` + para("") + `]}`), "content[0].content[0].type"},
		{"empty tableCell", wrapS(table(`{"type":"tableCell"}`)), "content[0].content[0].content[0].content"},
		{"colspan 2", wrapS(table(`{"type":"tableCell","attrs":{"colspan":2},"content":[` + para("") + `]}`)), "attrs.colspan"},
		{"rowspan 2", wrapS(table(`{"type":"tableHeader","attrs":{"rowspan":2},"content":[` + para("") + `]}`)), "attrs.rowspan"},
		{"colwidth 39", wrapS(table(cellWidth("[39]"))), "attrs.colwidth"},
		{"colwidth 2001", wrapS(table(cellWidth("[2001]"))), "attrs.colwidth"},
		{"colwidth two", wrapS(table(cellWidth("[100,100]"))), "attrs.colwidth"},
		{"colwidth empty", wrapS(table(cellWidth("[]"))), "attrs.colwidth"},
		{"colwidth number", wrapS(table(cellWidth("100"))), "attrs.colwidth"},
		{"colwidth fraction", wrapS(table(cellWidth("[100.5]"))), "attrs.colwidth"},

		{"image without assetId", wrapS(`{"type":"image"}`), "content[0].attrs.assetId"},
		{"image empty assetId", wrapS(`{"type":"image","attrs":{"assetId":""}}`), "attrs.assetId"},
		{"image numeric assetId", wrapS(`{"type":"image","attrs":{"assetId":7}}`), "attrs.assetId"},
		{"image alt too long", wrapS(`{"type":"image","attrs":{"assetId":"A","alt":"` + strings.Repeat("я", 201) + `"}}`), "attrs.alt"},

		{"details without detailsContent", wrapS(`{"type":"details","content":[{"type":"detailsSummary"}]}`), "content[0].content"},
		{"details without content", wrapS(`{"type":"details"}`), "content[0].content"},
		{"details wrong order", wrapS(`{"type":"details","content":[{"type":"detailsContent","content":[` + para("") + `]},{"type":"detailsSummary"}]}`), "content[0].content[0].type"},
		{"details extra child", wrapS(`{"type":"details","content":[` + detailsBody() + `,` + para("") + `]}`), "content[0].content"},
		{"empty detailsContent", wrapS(`{"type":"details","content":[{"type":"detailsSummary"},{"type":"detailsContent"}]}`), "content[0].content[1].content"},
		{"detailsSummary with block", wrapS(`{"type":"details","content":[{"type":"detailsSummary","content":[` + para("") + `]},{"type":"detailsContent","content":[` + para("") + `]}]}`), "content[0].content[0].content[0].type"},

		{"mention unknown kind", wrapS(para(mention("actor", ulid1, ""))), "attrs.kind"},
		{"mention without kind", wrapS(para(`{"type":"mention","attrs":{"id":"` + ulid1 + `"}}`)), "attrs.kind"},
		{"mention lowercase id", wrapS(para(mention("page", strings.ToLower(ulid1), ""))), "attrs.id"},
		{"mention short id", wrapS(para(mention("page", "ABC", ""))), "attrs.id"},
		{"mention without id", wrapS(para(`{"type":"mention","attrs":{"kind":"page"}}`)), "attrs.id"},
		{"mention label too long", wrapS(para(mention("page", ulid1, strings.Repeat("я", 201)))), "attrs.label"},
		{"mention without attrs", wrapS(para(`{"type":"mention"}`)), "attrs.kind"},

		{"inlineRoll incomplete", wrapS(para(roll("2d"))), "attrs.formula"},
		{"inlineRoll empty", wrapS(para(roll(""))), "attrs.formula"},
		{"inlineRoll without formula", wrapS(para(`{"type":"inlineRoll"}`)), "attrs.formula"},
		{"inlineRoll too long", wrapS(para(roll(strings.Repeat("1+", 100) + "1"))), "attrs.formula"},

		{"link javascript", wrapS(para(link("javascript:alert(1)"))), "marks[0].attrs.href"},
		{"link data", wrapS(para(link("data:text/html,x"))), "marks[0].attrs.href"},
		{"link relative", wrapS(para(link("/a/b"))), "marks[0].attrs.href"},
		{"link empty", wrapS(para(link(""))), "marks[0].attrs.href"},
		{"link without href", wrapS(para(`{"type":"text","text":"a","marks":[{"type":"link"}]}`)), "marks[0].attrs.href"},
		{"link unparsable", wrapS(para(link("http://a b\\u0001"))), "marks[0].attrs.href"},
		{"link too long", wrapS(para(link("https://x/" + strings.Repeat("a", 2000)))), "marks[0].attrs.href"},

		{"malformed JSON", `{"type":`, "<root>"},
		{"empty input", ``, "<root>"},
		{"trailing data", string(wrap(para(""))) + `{}`, "<root>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Validate([]byte(tt.doc), opts())
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tt.path) {
				t.Errorf("err = %q, want path %q", err, tt.path)
			}
		})
	}
}

// wrapS is wrap returning a string.
func wrapS(blocks string) string { return string(wrap(blocks)) }

func li() string { return `{"type":"listItem","content":[` + para("") + `]}` }

func detailsBody() string {
	return `{"type":"detailsSummary"},{"type":"detailsContent","content":[` + para("") + `]}`
}

func table(cell string) string {
	return `{"type":"table","content":[{"type":"tableRow","content":[` + cell + `]}]}`
}

func cellWidth(w string) string {
	return `{"type":"tableCell","attrs":{"colwidth":` + w + `},"content":[` + para("") + `]}`
}

func mention(kind, id, label string) string {
	return `{"type":"mention","attrs":{"kind":"` + kind + `","id":"` + id + `","label":"` + label + `"}}`
}

func roll(f string) string { return `{"type":"inlineRoll","attrs":{"formula":"` + f + `"}}` }

func link(href string) string {
	return `{"type":"text","text":"a","marks":[{"type":"link","attrs":{"href":"` + href + `"}}]}`
}

func TestValidateAccepted(t *testing.T) {
	tests := []struct {
		name string
		doc  string
		want string // normalized JSON
	}{
		{"mark on hardBreak is dropped", wrapS(para(`{"type":"hardBreak","marks":[{"type":"bold"}]}`)),
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"hardBreak"}]}]}`},
		{"marks on a non-text node are not validated", wrapS(para(`{"type":"hardBreak","marks":[{"type":"sparkle"}]}`)),
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"hardBreak"}]}]}`},
		{"uppercase link scheme", wrapS(para(link("HTTPS://x"))),
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"HTTPS://x"}}],"text":"a"}]}]}`},
		{"mailto link", wrapS(para(link("mailto:a@b.c"))),
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"mailto:a@b.c"}}],"text":"a"}]}]}`},
		{"heading level as 2.0", wrapS(`{"type":"heading","attrs":{"level":2.0}}`),
			`{"type":"doc","content":[{"type":"heading","attrs":{"level":2}}]}`},
		{"heading level as exponent", wrapS(`{"type":"heading","attrs":{"level":1e0}}`),
			`{"type":"doc","content":[{"type":"heading","attrs":{"level":1}}]}`},
		{"empty marks array", wrapS(para(`{"type":"text","text":"a","marks":[]}`)),
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]}]}`},
		{"empty content array", wrapS(`{"type":"paragraph","content":[]}`),
			`{"type":"doc","content":[{"type":"paragraph"}]}`},
		{"surrounding whitespace", " \n" + wrapS(`{"type":"paragraph"}`) + "\n",
			`{"type":"doc","content":[{"type":"paragraph"}]}`},
		{"colwidth boundaries", wrapS(table(cellWidth("[40]")) + `,` + table(cellWidth("[2000]"))),
			`{"type":"doc","content":[{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":1,"colwidth":[40],"rowspan":1},"content":[{"type":"paragraph"}]}]}]},{"type":"table","content":[{"type":"tableRow","content":[{"type":"tableCell","attrs":{"colspan":1,"colwidth":[2000],"rowspan":1},"content":[{"type":"paragraph"}]}]}]}]}`},
		{"image with 200 code point alt", wrapS(`{"type":"image","attrs":{"assetId":"A","alt":"` + strings.Repeat("я", 200) + `"}}`),
			`{"type":"doc","content":[{"type":"image","attrs":{"alt":"` + strings.Repeat("я", 200) + `","assetId":"A"}}]}`},
		{"orderedList start bounds", wrapS(`{"type":"orderedList","attrs":{"start":10000},"content":[` + li() + `]}`),
			`{"type":"doc","content":[{"type":"orderedList","attrs":{"start":10000},"content":[{"type":"listItem","content":[{"type":"paragraph"}]}]}]}`},
		{"listItem with block after paragraph", wrapS(`{"type":"bulletList","content":[{"type":"listItem","content":[` + para("") + `,{"type":"horizontalRule"}]}]}`),
			`{"type":"doc","content":[{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph"},{"type":"horizontalRule"}]}]}]}`},
		{"inline roll formula", wrapS(para(roll("4d6kh3"))),
			`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"inlineRoll","attrs":{"formula":"4d6kh3"}}]}]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := Validate([]byte(tt.doc), opts())
			if err != nil {
				t.Fatal(err)
			}
			if string(res.JSON) != tt.want {
				t.Errorf("JSON = %s\nwant   %s", res.JSON, tt.want)
			}
		})
	}
}

func TestValidateAssetOK(t *testing.T) {
	img := func(id string) string { return `{"type":"image","attrs":{"assetId":"` + id + `"}}` }

	t.Run("called once per distinct id after the walk", func(t *testing.T) {
		var calls []string
		o := Options{AssetOK: func(id string) (bool, error) {
			calls = append(calls, id)
			return true, nil
		}}
		res, err := Validate(wrap(img("B")+","+img("A")+","+img("B")+","+img("A")), o)
		if err != nil {
			t.Fatal(err)
		}
		if want := []string{"B", "A"}; !slices.Equal(calls, want) {
			t.Errorf("calls = %v, want %v", calls, want)
		}
		if want := []string{"B", "A"}; !slices.Equal(res.Assets, want) {
			t.Errorf("Assets = %v, want %v", res.Assets, want)
		}
	})

	t.Run("not an asset of the campaign", func(t *testing.T) {
		o := Options{AssetOK: func(id string) (bool, error) { return id != "BAD", nil }}
		_, err := Validate(wrap(img("A")+","+img("BAD")), o)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if !strings.Contains(err.Error(), "content[1].attrs.assetId") {
			t.Errorf("err = %q, want the path of the image", err)
		}
	})

	t.Run("lookup error is returned as is", func(t *testing.T) {
		boom := errors.New("db down")
		o := Options{AssetOK: func(string) (bool, error) { return false, boom }}
		_, err := Validate(wrap(img("A")), o)
		if !errors.Is(err, boom) {
			t.Fatalf("err = %v, want %v", err, boom)
		}
		if errors.Is(err, ErrInvalid) {
			t.Error("lookup error must not be ErrInvalid")
		}
	})

	t.Run("not called for a document with a syntax error", func(t *testing.T) {
		called := false
		o := Options{AssetOK: func(string) (bool, error) { called = true; return true, nil }}
		_, err := Validate(wrap(img("A")+`,{"type":"video"}`), o)
		if !errors.Is(err, ErrInvalid) || called {
			t.Errorf("err = %v, called = %v", err, called)
		}
	})

	t.Run("nil AssetOK panics", func(t *testing.T) {
		defer func() {
			if recover() == nil {
				t.Error("no panic")
			}
		}()
		_, _ = Validate(wrap(para("")), Options{})
	})
}

func TestValidateHandout(t *testing.T) {
	h := Options{Handout: true, AssetOK: anyAsset}
	images := func(n int) []byte {
		img := `{"type":"image","attrs":{"assetId":"A"}}`
		return wrap(strings.TrimSuffix(strings.Repeat(img+",", n), ","))
	}

	for name, doc := range map[string]string{
		"mention":    wrapS(para(mention("page", ulid1, ""))),
		"inlineRoll": wrapS(para(roll("1d20"))),
	} {
		t.Run(name+" is forbidden", func(t *testing.T) {
			_, err := Validate([]byte(doc), h)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if _, err := Validate([]byte(doc), opts()); err != nil {
				t.Errorf("a page must allow it: %v", err)
			}
		})
	}

	t.Run("20 images", func(t *testing.T) {
		if _, err := Validate(images(MaxHandoutImages), h); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("21 images", func(t *testing.T) {
		_, err := Validate(images(MaxHandoutImages+1), h)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("err = %v, want ErrInvalid", err)
		}
		if _, err := Validate(images(MaxHandoutImages+1), opts()); err != nil {
			t.Errorf("a page has no image limit: %v", err)
		}
	})
	t.Run("size limit", func(t *testing.T) {
		pad := func(n int) []byte {
			base := len(wrap(para(txt(""))))
			return wrap(para(txt(strings.Repeat("a", n-base))))
		}
		if _, err := Validate(pad(MaxHandoutBytes), h); err != nil {
			t.Fatalf("exactly the limit: %v", err)
		}
		_, err := Validate(pad(MaxHandoutBytes+1), h)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "larger") {
			t.Fatalf("err = %v, want a size error", err)
		}
		if _, err := Validate(pad(MaxHandoutBytes+1), opts()); err != nil {
			t.Errorf("a page may be that large: %v", err)
		}
	})
}

func TestValidateLimits(t *testing.T) {
	// nested builds a document of the given depth (the doc node is depth 1)
	// out of blockquotes ending in a paragraph.
	nested := func(depth int) []byte {
		n := depth - 2 // doc and the last paragraph
		var sb strings.Builder
		sb.WriteString(`{"type":"doc","content":[`)
		for i := 0; i < n; i++ {
			sb.WriteString(`{"type":"blockquote","content":[`)
		}
		sb.WriteString(`{"type":"paragraph"}`)
		for i := 0; i < n; i++ {
			sb.WriteString(`]}`)
		}
		sb.WriteString(`]}`)
		return []byte(sb.String())
	}

	t.Run("depth 50", func(t *testing.T) {
		if _, err := Validate(nested(MaxDepth), opts()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("depth 51", func(t *testing.T) {
		_, err := Validate(nested(MaxDepth+1), opts())
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "deeper") {
			t.Fatalf("err = %v, want a depth error", err)
		}
	})

	// A document of MaxNodes+1 nodes is just over MaxPageBytes, so the node
	// limit is exercised on the parsed tree directly.
	walk := func(n int) error {
		raw := wrap(strings.TrimSuffix(strings.Repeat(`{"type":"paragraph"},`, n-1), ","))
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			t.Fatal(err)
		}
		w := &walker{opts: opts(), seenMen: map[Ref]bool{}, seenAss: map[string]bool{}}
		_, err := w.node(v, "", 1, map[string]bool{"doc": true})
		return err
	}
	t.Run("50000 nodes", func(t *testing.T) {
		if err := walk(MaxNodes); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("50001 nodes", func(t *testing.T) {
		err := walk(MaxNodes + 1)
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "nodes") {
			t.Fatalf("err = %v, want a node count error", err)
		}
	})

	t.Run("size is checked before parsing", func(t *testing.T) {
		raw := bytes.Repeat([]byte("{"), MaxPageBytes+1)
		_, err := Validate(raw, opts())
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "larger") {
			t.Fatalf("err = %v, want a size error, not a syntax error", err)
		}
	})
	t.Run("page of exactly the limit", func(t *testing.T) {
		base := len(wrap(para(txt(""))))
		raw := wrap(para(txt(strings.Repeat("a", MaxPageBytes-base))))
		if len(raw) != MaxPageBytes {
			t.Fatalf("len = %d", len(raw))
		}
		if _, err := Validate(raw, opts()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestValidateExtracts(t *testing.T) {
	raw := wrap(strings.Join([]string{
		`{"type":"heading","attrs":{"level":1},"content":[` + txt("Таверна") + `]}`,
		para(txt("Хозяйка — "), mention("page", ulid1, "Мирта"), txt(". "), mention("scene", ulid2, "Зал"), txt(" "), mention("page", ulid1, "Мирта")),
		para(txt("Бросок "), roll("2d6+3"), `{"type":"hardBreak"}`, txt("дальше")),
		`{"type":"codeBlock","content":[` + txt("x = 1") + `]}`,
		`{"type":"details","content":[{"type":"detailsSummary","content":[` + txt("Секрет") + `]},{"type":"detailsContent","content":[` + para(txt("внутри")) + `]}]}`,
		`{"type":"image","attrs":{"assetId":"B"}}`,
		`{"type":"blockquote","content":[` + para("") + `]}`,
		`{"type":"image","attrs":{"assetId":"A"}}`,
		`{"type":"image","attrs":{"assetId":"B"}}`,
	}, ","))
	res, err := Validate(raw, opts())
	if err != nil {
		t.Fatal(err)
	}
	wantText := "Таверна\nХозяйка — Мирта. Зал Мирта\nБросок 2d6+3 дальше\nx = 1\nСекрет\nвнутри"
	if res.Text != wantText {
		t.Errorf("Text = %q\nwant   %q", res.Text, wantText)
	}
	wantMentions := []Ref{{KindPage, ulid1}, {KindScene, ulid2}}
	if !slices.Equal(res.Mentions, wantMentions) {
		t.Errorf("Mentions = %v, want %v", res.Mentions, wantMentions)
	}
	if want := []string{"B", "A"}; !slices.Equal(res.Assets, want) {
		t.Errorf("Assets = %v, want %v", res.Assets, want)
	}
}

func TestTextExample(t *testing.T) {
	raw := wrap(`{"type":"heading","attrs":{"level":1},"content":[` + txt("Таверна") + `]},` +
		para(txt("Хозяйка — "), mention("page", ulid1, "Мирта"), txt(".")) + `,{"type":"paragraph"}`)
	res, err := Validate(raw, opts())
	if err != nil {
		t.Fatal(err)
	}
	if want := "Таверна\nХозяйка — Мирта."; res.Text != want {
		t.Errorf("Text = %q, want %q", res.Text, want)
	}
}

func TestValidateEmptyResultSlices(t *testing.T) {
	res, err := Validate(wrap(para("")), opts())
	if err != nil {
		t.Fatal(err)
	}
	if res.Mentions == nil || res.Assets == nil || len(res.Mentions) != 0 || len(res.Assets) != 0 {
		t.Errorf("Mentions = %#v, Assets = %#v, want empty non-nil slices", res.Mentions, res.Assets)
	}
	if res.Text != "" {
		t.Errorf("Text = %q, want empty", res.Text)
	}
}

func TestEmpty(t *testing.T) {
	e := Empty()
	res, err := Validate(e, opts())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(res.JSON, e) {
		t.Errorf("Validate(Empty()) = %s, want %s", res.JSON, e)
	}
	e[0] = 'X'
	if e2 := Empty(); e2[0] != '{' {
		t.Error("Empty() shares its slice between calls")
	}
}

func TestValidIcon(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{"empty", "", true},
		{"emoji", "🍺", true},
		{"8 code points", "12345678", true},
		{"9 code points", "123456789", false},
		{"newline", "\n", false},
		{"tab inside", "a\tb", false},
		{"invalid UTF-8", "\xff", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidIcon(tt.s); got != tt.want {
				t.Errorf("ValidIcon(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
