package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/balfag/dnd-vtt/internal/asset"
	"github.com/balfag/dnd-vtt/internal/auth"
	"github.com/balfag/dnd-vtt/internal/game"
)

const (
	maxKindBytes = 32
	// uploadOverhead is room for the headers of the multipart parts.
	uploadOverhead = 1 << 20
)

type assetJSON struct {
	ID           string `json:"id"`
	CampaignID   string `json:"campaignId"`
	Kind         string `json:"kind"`
	MIME         string `json:"mime"`
	Size         int64  `json:"size"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	OriginalName string `json:"originalName"`
	CreatedAt    int64  `json:"createdAt"`
	URL          string `json:"url"` // "/assets/<id>"
}

func newAssetJSON(a asset.Asset) assetJSON {
	return assetJSON{
		ID:           a.ID,
		CampaignID:   a.CampaignID,
		Kind:         a.Kind,
		MIME:         a.MIME,
		Size:         a.Size,
		Width:        a.Width,
		Height:       a.Height,
		OriginalName: a.OriginalName,
		CreatedAt:    a.CreatedAt.UnixMilli(),
		URL:          "/assets/" + a.ID,
	}
}

// assetError answers with the response matching an asset.Service error.
func (a *api) assetError(w http.ResponseWriter, action string, err error) {
	var tooBig *http.MaxBytesError
	switch {
	case errors.Is(err, asset.ErrTooLarge), errors.As(err, &tooBig):
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "file is too large")
	case errors.Is(err, asset.ErrUnsupportedType):
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "unsupported file type")
	case errors.Is(err, asset.ErrInvalidImage), errors.Is(err, asset.ErrInvalidKind):
		writeError(w, http.StatusBadRequest, "bad_request", "invalid image or kind")
	case errors.Is(err, asset.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, asset.ErrInUse):
		writeError(w, http.StatusConflict, "conflict", "the asset is in use")
	default:
		a.internalError(w, action, err)
	}
}

// uploadAsset reads the multipart body as a stream: "kind" (optional) must
// come before "file"; nothing after "file" is read.
func (a *api) uploadAsset(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, a.d.Assets.MaxSize()+uploadOverhead)
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "multipart/form-data expected")
		return
	}

	kind := ""
	for {
		part, err := mr.NextPart()
		if err != nil {
			var tooBig *http.MaxBytesError
			if errors.As(err, &tooBig) {
				a.assetError(w, "read upload", err)
				return
			}
			// io.EOF here: the body ended without a "file" part.
			writeError(w, http.StatusBadRequest, "bad_request", "part \"file\" is required")
			return
		}

		switch part.FormName() {
		case "kind":
			b, err := io.ReadAll(io.LimitReader(part, maxKindBytes+1))
			if err != nil || len(b) > maxKindBytes {
				writeError(w, http.StatusBadRequest, "bad_request", "invalid kind")
				return
			}
			kind = string(b)
		case "file":
			status := http.StatusCreated
			saved, created, err := a.d.Assets.Upload(r.Context(), r.PathValue("id"), part.FileName(), kind, part)
			if err != nil {
				a.assetError(w, "upload asset", err)
				return
			}
			if !created {
				status = http.StatusOK
			}
			writeJSON(w, status, newAssetJSON(saved))
			return
		default:
			writeError(w, http.StatusBadRequest, "bad_request", "unknown part before \"file\"")
			return
		}
	}
}

func (a *api) listAssets(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	list, err := a.d.Assets.List(r.Context(), r.PathValue("id"))
	if err != nil {
		a.assetError(w, "list assets", err)
		return
	}
	out := make([]assetJSON, 0, len(list))
	for _, as := range list {
		out = append(out, newAssetJSON(as))
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *api) deleteAsset(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.requireGM(w, r); !ok {
		return
	}
	if err := a.d.Assets.Delete(r.Context(), r.PathValue("id")); err != nil {
		a.assetError(w, "delete asset", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// canReadAsset decides who may read an asset: the GM any, a player the
// background of the active scene of the campaign of the game their login
// session belongs to, and the image of a token of that scene they may see.
func (a *api) canReadAsset(ctx context.Context, acc auth.Account, sess auth.Session, as asset.Asset) (bool, error) {
	if acc.IsGM {
		return true, nil
	}
	gs, err := a.d.Game.ActiveGameSession(ctx)
	if errors.Is(err, game.ErrNoActiveGame) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if gs.ID != sess.GameSessionID || gs.CampaignID != as.CampaignID {
		return false, nil
	}
	isBackground, err := a.d.VTT.BackgroundOfActiveScene(ctx, as.CampaignID, as.ID)
	if err != nil || isBackground {
		return isBackground, err
	}
	p, err := a.d.Game.PlayerForAccount(ctx, as.CampaignID, acc.ID)
	if errors.Is(err, game.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if p.Removed {
		return false, nil
	}
	return a.d.VTT.PlayerCanSeeTokenImage(ctx, as.CampaignID, p.ID, as.ID)
}

// serveAsset answers 404 for an asset the account may not read, so the
// existence of the file is not confirmed.
func (a *api) serveAsset(w http.ResponseWriter, r *http.Request) {
	acc, sess, ok := a.requireSession(w, r)
	if !ok {
		return
	}
	as, err := a.d.Assets.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, asset.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	if err != nil {
		a.internalError(w, "get asset", err)
		return
	}
	allowed, err := a.canReadAsset(r.Context(), acc, sess, as)
	if err != nil {
		a.internalError(w, "check asset access", err)
		return
	}
	if !allowed {
		writeError(w, http.StatusNotFound, "not_found", "not found")
		return
	}

	f, err := a.d.Assets.Open(as)
	if err != nil {
		a.internalError(w, "open asset file", err)
		return
	}
	// Closed before the handler returns: Windows cannot delete an open file.
	defer func() { _ = f.Close() }()

	// The headers go first: ServeContent would guess the type itself otherwise.
	h := w.Header()
	h.Set("Content-Type", as.MIME)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "sandbox")
	h.Set("Cache-Control", "private, max-age=31536000, immutable")
	h.Set("ETag", `"`+as.SHA256+`"`)
	http.ServeContent(w, r, "", time.Time{}, f)
}
