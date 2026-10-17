// Package asset stores campaign files on disk by sha256 and their metadata in SQLite.
package asset

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"github.com/balfag/dnd-vtt/internal/id"
)

var (
	ErrNotFound        = errors.New("asset: not found") // asset or campaign
	ErrInUse           = errors.New("asset: in use")    // a foreign key still refers to it
	ErrTooLarge        = errors.New("asset: file too large")
	ErrUnsupportedType = errors.New("asset: unsupported file type")
	ErrInvalidImage    = errors.New("asset: invalid image") // unreadable header or size out of 1..16384
	ErrInvalidKind     = errors.New("asset: invalid kind")
)

const (
	DefaultMaxSize = 50 << 20 // bytes
	MaxDimension   = 16384
	KindOther      = "other"

	maxNameRunes = 200
	defaultName  = "file"
	sniffLen     = 512
)

var kinds = map[string]bool{"map": true, "token": true, "portrait": true, "handout": true, KindOther: true}

type Asset struct {
	ID, CampaignID, SHA256, MIME string
	Size                         int64
	Width, Height                int
	OriginalName, Kind           string
	CreatedAt                    time.Time
}

type Options struct {
	Now     func() time.Time // nil means time.Now
	MaxSize int64            // 0 means DefaultMaxSize
}

// Service is safe for concurrent use.
type Service struct {
	db      *sql.DB
	dataDir string
	now     func() time.Time
	maxSize int64
	locks   keyLock // per campaign and sha256: Upload and Delete of the same file
}

func NewService(db *sql.DB, dataDir string, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	maxSize := opts.MaxSize
	if maxSize <= 0 {
		maxSize = DefaultMaxSize
	}
	return &Service{db: db, dataDir: dataDir, now: now, maxSize: maxSize}
}

// MaxSize returns the file size limit in bytes.
func (s *Service) MaxSize() int64 { return s.maxSize }

const assetSelect = `SELECT id, campaign_id, sha256, mime, size, width, height, original_name, kind, created_at FROM asset`

type rowScanner interface{ Scan(dest ...any) error }

func scanAsset(r rowScanner) (Asset, error) {
	var (
		a         Asset
		createdMs int64
	)
	err := r.Scan(&a.ID, &a.CampaignID, &a.SHA256, &a.MIME, &a.Size, &a.Width, &a.Height, &a.OriginalName, &a.Kind, &createdMs)
	if err != nil {
		return Asset{}, err
	}
	a.CreatedAt = time.UnixMilli(createdMs)
	return a, nil
}

func (s *Service) assetsDir(campaignID string) string {
	return filepath.Join(s.dataDir, "campaigns", campaignID, "assets")
}

// Path returns <dataDir>/campaigns/<campaignID>/assets/<sha256>.<ext>.
func (s *Service) Path(a Asset) string {
	ext := ""
	if t, ok := typeByMIME(a.MIME); ok {
		ext = t.ext
	}
	return filepath.Join(s.assetsDir(a.CampaignID), a.SHA256+"."+ext)
}

// Open opens the stored file of a. The caller closes it.
func (s *Service) Open(a Asset) (*os.File, error) {
	return os.Open(s.Path(a))
}

func (s *Service) campaignExists(ctx context.Context, campaignID string) error {
	var one int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM campaign WHERE id = ?`, campaignID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find campaign: %w", err)
	}
	return nil
}

// cleanName keeps the part after the last / or \, trims spaces and cuts it to
// 200 runes; the name is only shown, never used as a path.
func cleanName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if utf8.RuneCountInString(name) > maxNameRunes {
		name = string([]rune(name)[:maxNameRunes])
	}
	if name == "" {
		return defaultName
	}
	return name
}

// Upload stores the file read from r for the campaign. kind "" means KindOther.
// It returns the asset and created=false when the campaign already has a file
// with the same sha256 (the existing asset, unchanged).
func (s *Service) Upload(ctx context.Context, campaignID, originalName, kind string, r io.Reader) (a Asset, created bool, err error) {
	if kind == "" {
		kind = KindOther
	}
	if !kinds[kind] {
		return Asset{}, false, ErrInvalidKind
	}
	if err := s.campaignExists(ctx, campaignID); err != nil {
		return Asset{}, false, err
	}

	dir := s.assetsDir(campaignID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Asset{}, false, fmt.Errorf("create assets dir: %w", err)
	}
	tmp, err := os.CreateTemp(dir, "upload-*")
	if err != nil {
		return Asset{}, false, fmt.Errorf("create temp file: %w", err)
	}
	// The temporary file goes away on every path that does not rename it.
	// It is closed first: Windows cannot remove an open file.
	closed, renamed := false, false
	defer func() {
		if !closed {
			_ = tmp.Close()
		}
		if !renamed {
			_ = os.Remove(tmp.Name())
		}
	}()

	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(r, s.maxSize+1))
	if err != nil {
		return Asset{}, false, fmt.Errorf("save upload: %w", err)
	}
	if size > s.maxSize {
		return Asset{}, false, ErrTooLarge
	}
	if size == 0 {
		return Asset{}, false, ErrInvalidImage
	}

	head := make([]byte, min(size, sniffLen))
	if _, err := tmp.ReadAt(head, 0); err != nil {
		return Asset{}, false, fmt.Errorf("read upload head: %w", err)
	}
	typ, ok := detectType(head)
	if !ok {
		return Asset{}, false, ErrUnsupportedType
	}
	width, height, ok := imageSize(tmp, typ)
	if !ok || width < 1 || width > MaxDimension || height < 1 || height > MaxDimension {
		return Asset{}, false, ErrInvalidImage
	}

	sum := hex.EncodeToString(hash.Sum(nil))
	// Held until return: a Delete of the same file must not remove it between
	// the rename and the insert.
	defer s.locks.lock(fileKey(campaignID, sum))()
	if existing, err := s.bySHA(ctx, campaignID, sum); err == nil {
		return existing, false, nil
	} else if !errors.Is(err, ErrNotFound) {
		return Asset{}, false, err
	}

	a = Asset{
		ID:           id.New(),
		CampaignID:   campaignID,
		SHA256:       sum,
		MIME:         typ.mime,
		Size:         size,
		Width:        width,
		Height:       height,
		OriginalName: cleanName(originalName),
		Kind:         kind,
		// Millisecond precision, as the row stores it: what Upload returns equals what Get does.
		CreatedAt: time.UnixMilli(s.now().UnixMilli()),
	}
	if err := tmp.Close(); err != nil {
		return Asset{}, false, fmt.Errorf("close upload: %w", err)
	}
	closed = true
	path := s.Path(a)
	if err := os.Rename(tmp.Name(), path); err != nil {
		// A concurrent upload of the same file may hold the target: the name
		// is the sha256, so what is there is the same content.
		if _, statErr := os.Stat(path); statErr != nil {
			return Asset{}, false, fmt.Errorf("store upload: %w", err)
		}
	} else {
		renamed = true
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO asset (id, campaign_id, sha256, mime, size, width, height, original_name, kind, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.CampaignID, a.SHA256, a.MIME, a.Size, a.Width, a.Height, a.OriginalName, a.Kind, a.CreatedAt.UnixMilli())
	if err != nil {
		if isConstraint(err, sqlite3.SQLITE_CONSTRAINT_UNIQUE) {
			// A parallel upload of the same file won: return its row.
			existing, getErr := s.bySHA(ctx, campaignID, sum)
			if getErr != nil {
				return Asset{}, false, getErr
			}
			return existing, false, nil
		}
		return Asset{}, false, fmt.Errorf("insert asset: %w", err)
	}
	return a, true, nil
}

func fileKey(campaignID, sum string) string { return campaignID + "/" + sum }

func (s *Service) bySHA(ctx context.Context, campaignID, sum string) (Asset, error) {
	a, err := scanAsset(s.db.QueryRowContext(ctx, assetSelect+` WHERE campaign_id = ? AND sha256 = ?`, campaignID, sum))
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, fmt.Errorf("find asset by sha256: %w", err)
	}
	return a, nil
}

// isConstraint reports whether err is a SQLite error with one of the codes.
func isConstraint(err error, codes ...int) bool {
	var serr *sqlite.Error
	if !errors.As(err, &serr) {
		return false
	}
	for _, c := range codes {
		if serr.Code() == c {
			return true
		}
	}
	return false
}

// List returns the campaign's assets, newest first. Unknown campaign: ErrNotFound.
func (s *Service) List(ctx context.Context, campaignID string) ([]Asset, error) {
	if err := s.campaignExists(ctx, campaignID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, assetSelect+` WHERE campaign_id = ? ORDER BY created_at DESC, id DESC`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []Asset{}
	for rows.Next() {
		a, err := scanAsset(rows)
		if err != nil {
			return nil, fmt.Errorf("scan asset: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list assets: %w", err)
	}
	return out, nil
}

func (s *Service) Get(ctx context.Context, id string) (Asset, error) {
	a, err := scanAsset(s.db.QueryRowContext(ctx, assetSelect+` WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Asset{}, ErrNotFound
	}
	if err != nil {
		return Asset{}, fmt.Errorf("get asset: %w", err)
	}
	return a, nil
}

// Delete removes the row and then the file. A row still referenced by a
// foreign key (ON DELETE RESTRICT) gives ErrInUse and keeps both.
func (s *Service) Delete(ctx context.Context, id string) error {
	a, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	// The key is taken before the transaction, never inside it.
	defer s.locks.lock(fileKey(a.CampaignID, a.SHA256))()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("delete asset: begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM asset WHERE id = ?`, id); err != nil {
		// RESTRICT is reported as a trigger constraint, not as a foreign key one.
		if isConstraint(err, sqlite3.SQLITE_CONSTRAINT_TRIGGER, sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY) {
			return ErrInUse
		}
		return fmt.Errorf("delete asset: %w", err)
	}
	if err := tx.Commit(); err != nil {
		if isConstraint(err, sqlite3.SQLITE_CONSTRAINT_TRIGGER, sqlite3.SQLITE_CONSTRAINT_FOREIGNKEY) {
			return ErrInUse
		}
		return fmt.Errorf("delete asset: commit: %w", err)
	}

	// The lock keeps a new row for this file from appearing, but the check is
	// cheap and guards against future changes. A row for the file means keep it.
	if _, err := s.bySHA(ctx, a.CampaignID, a.SHA256); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	// The row is gone, so a file that cannot be removed is only an orphan on
	// disk, and the service has no logger to say so: the error is dropped.
	_ = os.Remove(s.Path(a))
	return nil
}
