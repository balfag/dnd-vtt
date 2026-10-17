package asset

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/balfag/dnd-vtt/internal/store"
)

// webp1x1 is a 1x1 lossless WebP.
const webp1x1 = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

type env struct {
	t       *testing.T
	svc     *Service
	dataDir string
	camp    string
	clock   time.Time
	st      *store.Store
}

func newEnv(t *testing.T, opts Options) *env {
	t.Helper()
	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	e := &env{t: t, dataDir: t.TempDir(), clock: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), st: st}
	opts.Now = func() time.Time { return e.clock }
	e.svc = NewService(st.DB(), e.dataDir, opts)
	e.camp = e.campaign("Camp")
	return e
}

func (e *env) campaign(name string) string {
	e.t.Helper()
	id := "camp-" + name
	if _, err := e.st.DB().Exec(`INSERT INTO campaign (id, name, created_at) VALUES (?, ?, 1)`, id, name); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) upload(campaign, name, kind string, data []byte) (Asset, bool, error) {
	e.t.Helper()
	return e.svc.Upload(context.Background(), campaign, name, kind, bytes.NewReader(data))
}

func (e *env) mustUpload(name string, data []byte) Asset {
	e.t.Helper()
	a, _, err := e.upload(e.camp, name, "", data)
	if err != nil {
		e.t.Fatalf("upload %s: %v", name, err)
	}
	return a
}

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(0, 0, color.RGBA{R: 200, A: 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func jpegBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h)), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func gifBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := gif.Encode(&b, image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// pngHeader builds the signature and an IHDR chunk only: enough for DecodeConfig.
func pngHeader(width, height uint32) []byte {
	const ihdrLen = 13
	ihdr := make([]byte, ihdrLen)
	binary.BigEndian.PutUint32(ihdr[0:], width)
	binary.BigEndian.PutUint32(ihdr[4:], height)
	ihdr[8], ihdr[9] = 8, 6 // 8 bits, RGBA
	chunk := append([]byte("IHDR"), ihdr...)

	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	_ = binary.Write(&b, binary.BigEndian, uint32(ihdrLen))
	b.Write(chunk)
	_ = binary.Write(&b, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return b.Bytes()
}

// padded appends zero bytes after the image data (decoders ignore them) up to size.
func padded(t *testing.T, data []byte, size int) []byte {
	t.Helper()
	if len(data) > size {
		t.Fatalf("image of %d bytes does not fit %d", len(data), size)
	}
	return append(data, make([]byte, size-len(data))...)
}

func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (e *env) assertNoTempFiles() {
	e.t.Helper()
	matches, err := filepath.Glob(filepath.Join(e.dataDir, "campaigns", "*", "assets", "upload-*"))
	if err != nil || len(matches) != 0 {
		e.t.Errorf("temporary files left: %v, %v", matches, err)
	}
}

func TestUploadPNG(t *testing.T) {
	e := newEnv(t, Options{})
	data := pngBytes(t, 3, 2)

	a, created, err := e.upload(e.camp, "map.png", "map", data)
	if err != nil || !created {
		t.Fatalf("upload = %v, created %v", err, created)
	}
	if a.MIME != "image/png" || a.Width != 3 || a.Height != 2 || a.Size != int64(len(data)) ||
		a.SHA256 != sha(data) || a.Kind != "map" || a.OriginalName != "map.png" || a.CampaignID != e.camp ||
		!a.CreatedAt.Equal(e.clock) || a.ID == "" {
		t.Errorf("asset = %+v", a)
	}

	want := filepath.Join(e.dataDir, "campaigns", e.camp, "assets", a.SHA256+".png")
	if got := e.svc.Path(a); got != want {
		t.Errorf("Path = %q, want %q", got, want)
	}
	if got, err := fs.ReadFile(os.DirFS(filepath.Dir(want)), filepath.Base(want)); err != nil || !bytes.Equal(got, data) {
		t.Errorf("stored file differs: %v", err)
	}
	e.assertNoTempFiles()

	got, err := e.svc.Get(context.Background(), a.ID)
	if err != nil || got != a {
		t.Errorf("Get = %+v, %v; want %+v", got, err, a)
	}
}

func TestUploadOtherFormats(t *testing.T) {
	webp := mustBase64(t, webp1x1)
	tests := []struct {
		name          string
		data          []byte
		mime, ext     string
		width, height int
	}{
		{"jpeg", jpegBytes(t, 5, 4), "image/jpeg", "jpg", 5, 4},
		{"gif", gifBytes(t, 4, 3), "image/gif", "gif", 4, 3},
		{"webp", webp, "image/webp", "webp", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, Options{})
			a := e.mustUpload("file", tt.data)
			if a.MIME != tt.mime || a.Width != tt.width || a.Height != tt.height {
				t.Errorf("asset = %+v, want %s %dx%d", a, tt.mime, tt.width, tt.height)
			}
			if !strings.HasSuffix(e.svc.Path(a), "."+tt.ext) {
				t.Errorf("Path = %q, want extension %s", e.svc.Path(a), tt.ext)
			}
			if _, err := os.Stat(e.svc.Path(a)); err != nil {
				t.Error(err)
			}
		})
	}
}

func TestUploadIgnoresNameAndDeclaredType(t *testing.T) {
	e := newEnv(t, Options{})
	a := e.mustUpload("photo.txt", pngBytes(t, 2, 2))
	if a.MIME != "image/png" {
		t.Errorf("mime = %q, want image/png from the content", a.MIME)
	}
}

func TestUploadDeduplicates(t *testing.T) {
	e := newEnv(t, Options{})
	data := pngBytes(t, 3, 2)

	first, created, err := e.upload(e.camp, "a.png", "map", data)
	if err != nil || !created {
		t.Fatal(err)
	}
	again, created, err := e.upload(e.camp, "other-name.png", "token", data)
	if err != nil || created {
		t.Fatalf("repeat: %v, created %v; want created=false", err, created)
	}
	if again != first {
		t.Errorf("repeat returned %+v, want the unchanged %+v", again, first)
	}
	list, err := e.svc.List(context.Background(), e.camp)
	if err != nil || len(list) != 1 {
		t.Errorf("List = %v, %v; want one asset", list, err)
	}

	other := e.campaign("Other")
	third, created, err := e.upload(other, "a.png", "", data)
	if err != nil || !created || third.ID == first.ID {
		t.Errorf("same file in another campaign: %+v, created %v, %v; want a new asset", third, created, err)
	}
	e.assertNoTempFiles()
}

func TestUploadConcurrentSameFile(t *testing.T) {
	e := newEnv(t, Options{})
	data := pngBytes(t, 3, 2)

	const n = 8
	var wg sync.WaitGroup
	ids := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, _, err := e.upload(e.camp, "a.png", "", data)
			ids[i], errs[i] = a.ID, err
		}()
	}
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("upload %d: %v", i, errs[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("upload %d got asset %s, want %s", i, ids[i], ids[0])
		}
	}
	if list, _ := e.svc.List(context.Background(), e.camp); len(list) != 1 {
		t.Errorf("assets = %d, want 1", len(list))
	}
	e.assertNoTempFiles()
}

func TestUploadSizeLimit(t *testing.T) {
	e := newEnv(t, Options{MaxSize: 1024})
	small := pngBytes(t, 1, 1)

	_, _, err := e.upload(e.camp, "big.png", "", padded(t, small, 1025))
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("1025 bytes: err = %v, want ErrTooLarge", err)
	}
	e.assertNoTempFiles()

	if _, _, err := e.upload(e.camp, "edge.png", "", padded(t, small, 1024)); err != nil {
		t.Fatalf("exactly 1024 bytes: err = %v, want nil", err)
	}
	if e.svc.MaxSize() != 1024 {
		t.Errorf("MaxSize = %d", e.svc.MaxSize())
	}
	if got := NewService(nil, "", Options{}).MaxSize(); got != DefaultMaxSize {
		t.Errorf("default MaxSize = %d, want %d", got, DefaultMaxSize)
	}
}

func TestUploadRejects(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"text", []byte("just some text, not an image"), ErrUnsupportedType},
		{"svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"></svg>`), ErrUnsupportedType},
		{"pdf", []byte("%PDF-1.7\n1 0 obj\n"), ErrUnsupportedType},
		{"empty", nil, ErrInvalidImage},
		{"png with a cut header", []byte("\x89PNG\r\n\x1a\n\x00\x00"), ErrInvalidImage},
		{"png too wide", pngHeader(16385, 1), ErrInvalidImage},
		{"png too tall", pngHeader(1, 16385), ErrInvalidImage},
		{"png of zero width", pngHeader(0, 5), ErrInvalidImage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnv(t, Options{})
			_, _, err := e.upload(e.camp, "f", "", tt.data)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			e.assertNoTempFiles()
			if list, _ := e.svc.List(context.Background(), e.camp); len(list) != 0 {
				t.Errorf("a rejected file left rows: %v", list)
			}
		})
	}
}

func TestUploadKind(t *testing.T) {
	e := newEnv(t, Options{})
	if _, _, err := e.upload(e.camp, "a.png", "bogus", pngBytes(t, 2, 2)); !errors.Is(err, ErrInvalidKind) {
		t.Errorf("bogus kind: err = %v, want ErrInvalidKind", err)
	}
	e.assertNoTempFiles()

	a, _, err := e.upload(e.camp, "a.png", "", pngBytes(t, 2, 2))
	if err != nil || a.Kind != "other" {
		t.Errorf("empty kind: %+v, %v; want other", a, err)
	}
	for i, kind := range []string{"map", "token", "portrait", "handout", "other"} {
		data := pngBytes(t, 10+i, 2)
		if a, _, err := e.upload(e.camp, "a.png", kind, data); err != nil || a.Kind != kind {
			t.Errorf("kind %s: %+v, %v", kind, a, err)
		}
	}
}

func TestUploadOriginalName(t *testing.T) {
	e := newEnv(t, Options{})
	tests := []struct{ in, want string }{
		{"../../evil.png", "evil.png"},
		{`C:\x\evil.png`, "evil.png"},
		{"  spaced name.png  ", "spaced name.png"},
		{strings.Repeat("ж", 300), strings.Repeat("ж", 200)},
		{"", "file"},
		{"dir/", "file"},
		{"   ", "file"},
	}
	for i, tt := range tests {
		a, _, err := e.upload(e.camp, tt.in, "", pngBytes(t, 2+i, 2))
		if err != nil || a.OriginalName != tt.want {
			t.Errorf("name %q -> %q, %v; want %q", tt.in, a.OriginalName, err, tt.want)
		}
	}
}

func TestUploadUnknownCampaign(t *testing.T) {
	e := newEnv(t, Options{})
	if _, _, err := e.upload("nope", "a.png", "", pngBytes(t, 2, 2)); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.List(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("List: err = %v, want ErrNotFound", err)
	}
	if _, err := e.svc.Get(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get: err = %v, want ErrNotFound", err)
	}
	if err := e.svc.Delete(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete: err = %v, want ErrNotFound", err)
	}
}

func TestListNewestFirst(t *testing.T) {
	e := newEnv(t, Options{})
	list, err := e.svc.List(context.Background(), e.camp)
	if err != nil || list == nil || len(list) != 0 {
		t.Fatalf("empty List = %#v, %v; want an empty non-nil slice", list, err)
	}

	var ids []string
	for i := range 3 {
		e.clock = e.clock.Add(time.Minute)
		ids = append(ids, e.mustUpload("f", pngBytes(t, 2+i, 2)).ID)
	}
	list, err = e.svc.List(context.Background(), e.camp)
	if err != nil || len(list) != 3 {
		t.Fatalf("List = %v, %v", list, err)
	}
	for i, a := range list {
		if want := ids[len(ids)-1-i]; a.ID != want {
			t.Errorf("list[%d] = %s, want %s (newest first)", i, a.ID, want)
		}
	}
}

func TestDelete(t *testing.T) {
	e := newEnv(t, Options{})
	a := e.mustUpload("a.png", pngBytes(t, 2, 2))

	if err := e.svc.Delete(context.Background(), a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Get(context.Background(), a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want ErrNotFound", err)
	}
	if _, err := os.Stat(e.svc.Path(a)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("file after Delete: err = %v, want not exist", err)
	}

	// The same content can be uploaded again after a delete.
	if _, created, err := e.upload(e.camp, "a.png", "", pngBytes(t, 2, 2)); err != nil || !created {
		t.Errorf("re-upload: %v, created %v", err, created)
	}
}

func TestDeleteInUse(t *testing.T) {
	e := newEnv(t, Options{})
	a := e.mustUpload("a.png", pngBytes(t, 2, 2))
	for _, q := range []string{
		`CREATE TABLE ref (asset_id TEXT REFERENCES asset (id) ON DELETE RESTRICT) STRICT`,
	} {
		if _, err := e.st.DB().Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := e.st.DB().Exec(`INSERT INTO ref (asset_id) VALUES (?)`, a.ID); err != nil {
		t.Fatal(err)
	}

	if err := e.svc.Delete(context.Background(), a.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("err = %v, want ErrInUse", err)
	}
	if _, err := e.svc.Get(context.Background(), a.ID); err != nil {
		t.Errorf("the row must stay: %v", err)
	}
	if _, err := os.Stat(e.svc.Path(a)); err != nil {
		t.Errorf("the file must stay: %v", err)
	}
}

func TestDeleteCampaignCascades(t *testing.T) {
	e := newEnv(t, Options{})
	a := e.mustUpload("a.png", pngBytes(t, 2, 2))
	if _, err := e.st.DB().Exec(`DELETE FROM campaign WHERE id = ?`, e.camp); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.Get(context.Background(), a.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after the campaign is gone: err = %v, want ErrNotFound", err)
	}
}

func TestDeleteRacingUploadKeepsInvariant(t *testing.T) {
	e := newEnv(t, Options{})
	data := pngBytes(t, 3, 3)
	for i := range 50 {
		a := e.mustUpload("a.png", data)

		start := make(chan struct{})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			<-start
			if err := e.svc.Delete(context.Background(), a.ID); err != nil && !errors.Is(err, ErrNotFound) {
				t.Errorf("iteration %d: delete: %v", i, err)
			}
		}()
		go func() {
			defer wg.Done()
			<-start
			if _, _, err := e.upload(e.camp, "a.png", "", data); err != nil {
				t.Errorf("iteration %d: upload: %v", i, err)
			}
		}()
		close(start)
		wg.Wait()

		rows, err := e.svc.List(context.Background(), e.camp)
		if err != nil {
			t.Fatal(err)
		}
		_, statErr := os.Stat(e.svc.Path(a))
		switch {
		case len(rows) > 0 && statErr != nil:
			t.Fatalf("iteration %d: row exists but file is gone: %v", i, statErr)
		case len(rows) == 0 && statErr == nil:
			t.Fatalf("iteration %d: no row but the file is left", i)
		}
		for _, r := range rows {
			if err := e.svc.Delete(context.Background(), r.ID); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestAssetsDirPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on Windows")
	}
	e := newEnv(t, Options{})
	e.mustUpload("a.png", pngBytes(t, 2, 2))
	info, err := os.Stat(filepath.Join(e.dataDir, "campaigns", e.camp, "assets"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("assets dir mode = %o, want 700", got)
	}
}
