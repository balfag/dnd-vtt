package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/png"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/ws/protocol"
)

type multipartPart struct {
	name     string
	filename string // set for a file part
	data     []byte
}

// multipartBody builds a multipart/form-data body with the parts in the given order.
func multipartBody(t *testing.T, parts ...multipartPart) (string, string) {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for _, p := range parts {
		var (
			pw  interface{ Write([]byte) (int, error) }
			err error
		)
		if p.filename != "" {
			pw, err = w.CreateFormFile(p.name, p.filename)
		} else {
			pw, err = w.CreateFormField(p.name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pw.Write(p.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return w.FormDataContentType(), body.String()
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type assetFixture struct {
	*apiFixture
	gm         *http.Cookie
	campaignID string
}

func newAssetFixture(t *testing.T, opts asset.Options) *assetFixture {
	t.Helper()
	f := newAPIFixtureWithAssets(t, opts)
	gm := f.gmCookie()
	return &assetFixture{apiFixture: f, gm: gm, campaignID: f.createCampaign(gm, "Camp")}
}

// upload posts the parts as the given user.
func (f *assetFixture) upload(cookie *http.Cookie, parts ...multipartPart) *httptest.ResponseRecorder {
	f.t.Helper()
	contentType, body := multipartBody(f.t, parts...)
	return f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/assets", body,
		withHeader("Content-Type", contentType), withCookie(cookie))
}

func filePart(name string, data []byte) multipartPart {
	return multipartPart{name: "file", filename: name, data: data}
}

func kindPart(kind string) multipartPart { return multipartPart{name: "kind", data: []byte(kind)} }

func (f *assetFixture) mustUpload(name string, data []byte) assetJSON {
	f.t.Helper()
	rec := f.upload(f.gm, filePart(name, data))
	if rec.Code != http.StatusCreated {
		f.t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	var a assetJSON
	decodeBody(f.t, rec, &a)
	return a
}

func TestAssetUploadListDelete(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	data := testPNG(t, 3, 2)

	rec := f.upload(f.gm, kindPart("map"), filePart("tavern.png", data))
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload = %d %s", rec.Code, rec.Body.String())
	}
	var a assetJSON
	decodeBody(t, rec, &a)
	sum := sha256.Sum256(data)
	if a.Kind != "map" || a.MIME != "image/png" || a.Width != 3 || a.Height != 2 || a.Size != int64(len(data)) ||
		a.OriginalName != "tavern.png" || a.CampaignID != f.campaignID || a.URL != "/assets/"+a.ID || a.CreatedAt == 0 {
		t.Errorf("asset = %+v (sha %s)", a, hex.EncodeToString(sum[:]))
	}

	rec = f.upload(f.gm, kindPart("token"), filePart("again.png", data))
	var again assetJSON
	decodeBody(t, rec, &again)
	if rec.Code != http.StatusOK || again != a {
		t.Errorf("repeat = %d %+v, want 200 and the unchanged asset %+v", rec.Code, again, a)
	}

	rec = f.do(http.MethodGet, "/api/campaigns/"+f.campaignID+"/assets", "", withCookie(f.gm))
	var list []assetJSON
	decodeBody(t, rec, &list)
	if rec.Code != http.StatusOK || len(list) != 1 || list[0] != a {
		t.Errorf("list = %d %+v", rec.Code, list)
	}

	if rec := f.do(http.MethodDelete, "/api/assets/"+a.ID, "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := f.do(http.MethodGet, a.URL, "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("GET after delete = %d, want 404", rec.Code)
	}
	if rec := f.do(http.MethodDelete, "/api/assets/"+a.ID, "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("second delete = %d, want 404", rec.Code)
	}
}

func TestAssetListIsEmptyArray(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	rec := f.do(http.MethodGet, "/api/campaigns/"+f.campaignID+"/assets", "", withCookie(f.gm))
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("empty list = %q, want []", rec.Body.String())
	}
	if rec := f.do(http.MethodGet, "/api/campaigns/nope/assets", "", withCookie(f.gm)); rec.Code != http.StatusNotFound {
		t.Errorf("unknown campaign = %d, want 404", rec.Code)
	}
}

func TestAssetPlayerAndAnonymousAccess(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	a := f.mustUpload("a.png", testPNG(t, 2, 2))
	_, link := f.startGame(f.gm, f.campaignID)
	bob, _ := f.playerSession(link, "bob")

	contentType, body := multipartBody(t, filePart("b.png", testPNG(t, 3, 3)))
	post := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/assets", body,
		withHeader("Content-Type", contentType), withCookie(bob))
	if post.Code != http.StatusForbidden {
		t.Errorf("player upload = %d, want 403", post.Code)
	}
	if rec := f.do(http.MethodGet, "/api/campaigns/"+f.campaignID+"/assets", "", withCookie(bob)); rec.Code != http.StatusForbidden {
		t.Errorf("player list = %d, want 403", rec.Code)
	}
	if rec := f.do(http.MethodDelete, "/api/assets/"+a.ID, "", withCookie(bob)); rec.Code != http.StatusForbidden {
		t.Errorf("player delete = %d, want 403", rec.Code)
	}
	if rec := f.do(http.MethodGet, a.URL, "", withCookie(bob)); rec.Code != http.StatusNotFound {
		t.Errorf("player GET /assets = %d, want 404 (not 403: the file is not confirmed)", rec.Code)
	}

	if rec := f.do(http.MethodGet, a.URL, ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous GET /assets = %d, want 401", rec.Code)
	}
	if rec := f.do(http.MethodGet, "/api/campaigns/"+f.campaignID+"/assets", ""); rec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous list = %d, want 401", rec.Code)
	}
}

func TestServeAsset(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	data := testPNG(t, 3, 2)
	a := f.mustUpload("a.png", data)
	sum := sha256.Sum256(data)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`

	rec := f.do(http.MethodGet, a.URL, "", withCookie(f.gm))
	if rec.Code != http.StatusOK || !bytes.Equal(rec.Body.Bytes(), data) {
		t.Fatalf("GET = %d, %d bytes; want 200 and the file", rec.Code, rec.Body.Len())
	}
	for header, want := range map[string]string{
		"Content-Type":            "image/png",
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox",
		"Cache-Control":           "private, max-age=31536000, immutable",
		"ETag":                    etag,
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	rec = f.do(http.MethodGet, a.URL, "", withCookie(f.gm), withHeader("If-None-Match", etag))
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("If-None-Match = %d with %d bytes, want 304 and no body", rec.Code, rec.Body.Len())
	}
}

func TestServeAssetUnknown(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	for _, path := range []string{"/assets/does-not-exist", "/assets/..%2F..%2Fdndvtt.db"} {
		rec := f.do(http.MethodGet, path, "", withCookie(f.gm))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestAssetUploadErrors(t *testing.T) {
	f := newAssetFixture(t, asset.Options{MaxSize: 1024})
	small := testPNG(t, 1, 1)
	big := append(append([]byte{}, small...), make([]byte, 2000)...)

	tests := []struct {
		name  string
		parts []multipartPart
		want  int
		code  string
	}{
		{"file over the limit", []multipartPart{filePart("big.png", big)}, http.StatusRequestEntityTooLarge, "too_large"},
		{"svg", []multipartPart{filePart("x.svg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))}, http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{"text with a png name", []multipartPart{filePart("x.png", []byte("not an image"))}, http.StatusUnsupportedMediaType, "unsupported_media_type"},
		{"no file part", []multipartPart{kindPart("map")}, http.StatusBadRequest, "bad_request"},
		{"unknown part before file", []multipartPart{{name: "foo", data: []byte("x")}, filePart("a.png", small)}, http.StatusBadRequest, "bad_request"},
		{"kind too long", []multipartPart{kindPart(strings.Repeat("k", 40)), filePart("a.png", small)}, http.StatusBadRequest, "bad_request"},
		{"unknown kind", []multipartPart{kindPart("bogus"), filePart("a.png", small)}, http.StatusBadRequest, "bad_request"},
		{"broken image", []multipartPart{filePart("a.png", []byte("\x89PNG\r\n\x1a\n\x00"))}, http.StatusBadRequest, "bad_request"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := f.upload(f.gm, tt.parts...)
			if rec.Code != tt.want || errorCode(t, rec) != tt.code {
				t.Errorf("= %d %s, want %d %s", rec.Code, rec.Body.String(), tt.want, tt.code)
			}
		})
	}

	rec := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/assets", "{}", withCookie(f.gm))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("a JSON body = %d, want 400", rec.Code)
	}
	contentType, body := multipartBody(t, filePart("a.png", small))
	rec = f.do(http.MethodPost, "/api/campaigns/nope/assets", body, withHeader("Content-Type", contentType), withCookie(f.gm))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown campaign = %d, want 404", rec.Code)
	}
}

func TestAssetUploadKindAfterFileIsIgnored(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	rec := f.upload(f.gm, filePart("a.png", testPNG(t, 2, 2)), kindPart("map"))
	var a assetJSON
	decodeBody(t, rec, &a)
	if rec.Code != http.StatusCreated || a.Kind != "other" {
		t.Errorf("= %d, kind %q; want 201 and other (the part after the file is not read)", rec.Code, a.Kind)
	}
}

func TestAssetUploadChecksOrigin(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	contentType, body := multipartBody(t, filePart("a.png", testPNG(t, 2, 2)))
	rec := f.do(http.MethodPost, "/api/campaigns/"+f.campaignID+"/assets", body,
		withHeader("Content-Type", contentType), withHeader("Origin", "http://evil.example"), withCookie(f.gm))
	if rec.Code != http.StatusForbidden {
		t.Errorf("foreign Origin = %d, want 403", rec.Code)
	}
}

func TestPlayerReadsTheBackgroundOfTheActiveSceneOnly(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	ctx := context.Background()
	bgA := f.mustUpload("a.png", testPNG(t, 300, 200))
	bgB := f.mustUpload("b.png", testPNG(t, 301, 200))
	plain := f.mustUpload("c.png", testPNG(t, 302, 200))

	sceneA, err := f.vtt.CreateScene(ctx, f.campaignID, protocol.SceneCreate{Name: "A"}) // becomes active
	if err != nil {
		t.Fatal(err)
	}
	sceneB, err := f.vtt.CreateScene(ctx, f.campaignID, protocol.SceneCreate{Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	for scene, bg := range map[string]string{sceneA.SceneID: bgA.ID, sceneB.SceneID: bgB.ID} {
		if _, err := f.vtt.UpdateScene(ctx, f.campaignID, protocol.SceneUpdate{SceneID: scene, BackgroundAssetID: json.RawMessage(`"` + bg + `"`)}); err != nil {
			t.Fatal(err)
		}
	}
	gsID, link := f.startGame(f.gm, f.campaignID)
	bob, _ := f.playerSession(link, "bob")

	status := func(a assetJSON) int {
		return f.do(http.MethodGet, a.URL, "", withCookie(bob)).Code
	}
	if got := status(bgA); got != http.StatusOK {
		t.Errorf("background of the active scene = %d, want 200", got)
	}
	if got := status(bgB); got != http.StatusNotFound {
		t.Errorf("background of an inactive scene = %d, want 404", got)
	}
	if got := status(plain); got != http.StatusNotFound {
		t.Errorf("an asset that is no background = %d, want 404", got)
	}

	if _, err := f.vtt.ActivateScene(ctx, f.campaignID, sceneB.SceneID); err != nil {
		t.Fatal(err)
	}
	if got := status(bgA); got != http.StatusNotFound {
		t.Errorf("the old background after another scene was activated = %d, want 404", got)
	}
	if got := status(bgB); got != http.StatusOK {
		t.Errorf("the new background = %d, want 200", got)
	}

	// The GM still reads everything.
	if got := f.do(http.MethodGet, plain.URL, "", withCookie(f.gm)).Code; got != http.StatusOK {
		t.Errorf("GM reads any asset: %d", got)
	}

	// A login session of an ended game is gone: the player is signed out.
	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("end game = %d", rec.Code)
	}
	if got := status(bgB); got != http.StatusUnauthorized {
		t.Errorf("after the game ended = %d, want 401", got)
	}
}

func TestPlayerOfAnotherCampaignDoesNotReadTheBackground(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	ctx := context.Background()
	bg := f.mustUpload("a.png", testPNG(t, 300, 200))
	scene, err := f.vtt.CreateScene(ctx, f.campaignID, protocol.SceneCreate{Name: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.vtt.UpdateScene(ctx, f.campaignID, protocol.SceneUpdate{SceneID: scene.SceneID, BackgroundAssetID: json.RawMessage(`"` + bg.ID + `"`)}); err != nil {
		t.Fatal(err)
	}

	// The game runs in another campaign; its player must not see this background.
	otherID := f.createCampaign(f.gm, "Other")
	_, link := f.startGame(f.gm, otherID)
	bob, _ := f.playerSession(link, "bob")
	if got := f.do(http.MethodGet, bg.URL, "", withCookie(bob)).Code; got != http.StatusNotFound {
		t.Errorf("background of a campaign without a game = %d, want 404", got)
	}
}

func TestPlayerReadsImagesOfVisibleTokensOnly(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	ctx := context.Background()
	sceneA, err := f.vtt.CreateScene(ctx, f.campaignID, protocol.SceneCreate{Name: "A"}) // active
	if err != nil {
		t.Fatal(err)
	}
	sceneB, err := f.vtt.CreateScene(ctx, f.campaignID, protocol.SceneCreate{Name: "B"})
	if err != nil {
		t.Fatal(err)
	}
	_, link := f.startGame(f.gm, f.campaignID)
	bob, me := f.playerSession(link, "bob")
	if me.Player == nil {
		t.Fatal("bob has no participant")
	}

	x, y := 100.0, 100.0
	width := 20 // every image differs, or the upload would be a repeat
	token := func(scene string, name string, fields protocol.TokenFields) assetJSON {
		t.Helper()
		width++
		img := f.mustUpload(name+".png", testPNG(t, width, 20))
		fields.X, fields.Y = &x, &y
		fields.ImageAssetID = json.RawMessage(`"` + img.ID + `"`)
		if _, err := f.vtt.CreateToken(ctx, f.campaignID, protocol.TokenCreate{SceneID: scene, TokenFields: fields}); err != nil {
			t.Fatal(err)
		}
		return img
	}
	yes := true
	gmLayer := "gm"
	visible := token(sceneA.SceneID, "visible", protocol.TokenFields{})
	hidden := token(sceneA.SceneID, "hidden", protocol.TokenFields{Hidden: &yes})
	ownHidden := token(sceneA.SceneID, "own-hidden", protocol.TokenFields{Hidden: &yes, Owners: []string{me.Player.ID}})
	gmOnly := token(sceneA.SceneID, "gm-layer", protocol.TokenFields{Layer: &gmLayer})
	inactive := token(sceneB.SceneID, "inactive", protocol.TokenFields{})

	status := func(a assetJSON) int { return f.do(http.MethodGet, a.URL, "", withCookie(bob)).Code }
	for _, tt := range []struct {
		name string
		img  assetJSON
		want int
	}{
		{"visible token", visible, http.StatusOK},
		{"hidden token", hidden, http.StatusNotFound},
		{"own hidden token", ownHidden, http.StatusOK},
		{"token on the gm layer", gmOnly, http.StatusNotFound},
		{"token of an inactive scene", inactive, http.StatusNotFound},
	} {
		if got := status(tt.img); got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, got, tt.want)
		}
	}
	if got := f.do(http.MethodGet, hidden.URL, "", withCookie(f.gm)).Code; got != http.StatusOK {
		t.Errorf("GM reads the image of a hidden token: %d", got)
	}
}

func TestPlayerReadsImagesOfHandoutsShownToThem(t *testing.T) {
	f := newAssetFixture(t, asset.Options{})
	ctx := context.Background()
	gsID, link := f.startGame(f.gm, f.campaignID)
	bob, bobMe := f.playerSession(link, "bob")
	carl, _ := f.playerSession(link, "carl")
	if bobMe.Player == nil {
		t.Fatal("bob has no participant")
	}

	handout := func(name string, width int, to []string) assetJSON {
		t.Helper()
		img := f.mustUpload(name+".png", testPNG(t, width, 20))
		content := json.RawMessage(`{"type":"doc","content":[{"type":"image","attrs":{"assetId":"` + img.ID + `"}}]}`)
		if _, err := f.chat.SendHandout(ctx, f.campaignID, gsID, "", "Letter", content, to); err != nil {
			t.Fatal(err)
		}
		return img
	}
	public := handout("public", 21, nil)
	private := handout("private", 22, []string{bobMe.Player.ID})
	unshown := f.mustUpload("unshown.png", testPNG(t, 23, 20))

	status := func(a assetJSON, c *http.Cookie) int { return f.do(http.MethodGet, a.URL, "", withCookie(c)).Code }
	for _, tt := range []struct {
		name   string
		img    assetJSON
		cookie *http.Cookie
		want   int
	}{
		{"public handout, bob", public, bob, http.StatusOK},
		{"public handout, carl", public, carl, http.StatusOK},
		{"private handout, addressee", private, bob, http.StatusOK},
		{"private handout, another player", private, carl, http.StatusNotFound},
		{"an image that was never shown", unshown, bob, http.StatusNotFound},
		{"private handout, GM", private, f.gm, http.StatusOK},
	} {
		if got := status(tt.img, tt.cookie); got != tt.want {
			t.Errorf("%s = %d, want %d", tt.name, got, tt.want)
		}
	}

	// A player of another campaign does not read it even from a running game.
	if rec := f.do(http.MethodPost, "/api/game-sessions/"+gsID+"/end", "", withCookie(f.gm)); rec.Code != http.StatusNoContent {
		t.Fatalf("end game = %d", rec.Code)
	}
	if got := status(public, bob); got != http.StatusUnauthorized {
		t.Errorf("after the game ended = %d, want 401", got)
	}
	otherID := f.createCampaign(f.gm, "Other")
	_, otherLink := f.startGame(f.gm, otherID)
	eve, _ := f.playerSession(otherLink, "eve")
	if got := status(public, eve); got != http.StatusNotFound {
		t.Errorf("player of another campaign = %d, want 404", got)
	}
}
