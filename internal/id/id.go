// Package id generates identifiers for database rows.
package id

import "github.com/oklog/ulid/v2"

// New returns a new ULID string (26 characters, Crockford base32,
// sortable by creation time).
func New() string { return ulid.Make().String() }
