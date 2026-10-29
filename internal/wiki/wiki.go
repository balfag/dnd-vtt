// Package wiki stores the GM's wiki: pages, databases, tags, links and search.
package wiki

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	ErrNotFound = errors.New("wiki: not found") // page or campaign; pages in the trash too
	ErrInvalid  = errors.New("wiki: invalid")   // wrapped with details; doc.ErrInvalid is wrapped too
	ErrConflict = errors.New("wiki: conflict")  // stale version, page limit
)

const (
	MaxPages = 10000 // per campaign, trash and records included
	MaxDepth = 20    // a root page has depth 1
	MaxTitle = 200   // code points

	// maxWalk bounds the recursive queries so damaged data cannot loop.
	maxWalk = 64
)

// Kinds of a page.
const (
	KindPage     = "page"
	KindDatabase = "database"
)

type Options struct{ Now func() time.Time } // nil: time.Now

// Service is safe for concurrent use.
type Service struct {
	db  *sql.DB
	now func() time.Time
}

func NewService(db *sql.DB, opts Options) *Service {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &Service{db: db, now: now}
}

type PageNode struct {
	ID, ParentID, Kind, Title, Icon string // ParentID "" at the root
	Position, RecordCount           int    // RecordCount: live children of a database, else 0
}

type PageRef struct{ ID, Title, Icon, Kind string }

type Tag struct{ ID, Name string }

type RefInfo struct {
	Label  string
	Exists bool
}

type Page struct {
	ID, CampaignID, ParentID, Kind string
	IsRecord                       bool // the parent is a database
	Title, Icon, CoverAssetID      string
	Content, Props                 json.RawMessage
	Tags                           []Tag              // by name; never nil
	Path                           []PageRef          // ancestors from the root; never nil
	Refs                           map[string]RefInfo // key "<kind>:<id>"; never nil
	Version                        int
	CreatedAt, UpdatedAt           time.Time
}

type NewPage struct {
	ParentID string // "" for the root
	Title    string
	Icon     string
	Content  json.RawMessage // nil: doc.Empty()

	Kind     string          // "" or "page" | "database"
	Props    json.RawMessage // only when the parent is a database
	Database *NewDatabase    // only with Kind "database"
}

type SavePage struct {
	Version      int
	Title, Icon  string
	CoverAssetID string // "" removes the cover
	Content      json.RawMessage
	Force        bool
}

type Saved struct {
	Version   int
	UpdatedAt time.Time
	Refs      map[string]RefInfo
}
