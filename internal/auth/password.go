// Package auth implements local accounts, passwords and login sessions (ADR-0008).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// argon2id parameters from ADR-0008. VerifyPassword reads parameters from the
// encoded hash, so changing these does not invalidate stored passwords.
const (
	argonMemoryKiB = 64 * 1024
	argonTime      = 1
	argonThreads   = 4
	argonSaltLen   = 16
	argonKeyLen    = 32
)

var loginPattern = regexp.MustCompile(`^[a-z0-9_.-]{3,32}$`)

var errMalformedHash = errors.New("malformed password hash")

// HashPassword returns the argon2id hash of password in PHC format:
// $argon2id$v=19$m=65536,t=1,p=4$<salt>$<key>, salt and key in base64
// without padding (base64.RawStdEncoding). The salt comes from crypto/rand.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemoryKiB, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoryKiB, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether password matches encoded. A malformed
// encoded value is an error, not a mismatch. Keys are compared with
// crypto/subtle.ConstantTimeCompare.
func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" ||
		parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return false, errMalformedHash
	}

	var m, t, p uint32
	if n, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil || n != 3 {
		return false, errMalformedHash
	}
	// Sscanf ignores trailing characters, so compare the round trip.
	if m == 0 || t == 0 || p == 0 || p > 255 ||
		parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", m, t, p) {
		return false, errMalformedHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, errMalformedHash
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false, errMalformedHash
	}

	//nolint:gosec // G115: p is checked to be at most 255 above.
	got := argon2.IDKey([]byte(password), salt, t, m, uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// NormalizeLogin lower-cases login and checks it against ^[a-z0-9_.-]{3,32}$.
// It returns ErrInvalidLogin when the result does not match.
func NormalizeLogin(login string) (string, error) {
	login = strings.ToLower(login)
	if !loginPattern.MatchString(login) {
		return "", ErrInvalidLogin
	}
	return login, nil
}

// ValidatePassword checks that password has 8 to 128 characters (runes).
// It returns ErrInvalidPassword otherwise.
func ValidatePassword(password string) error {
	if n := utf8.RuneCountInString(password); n < 8 || n > 128 {
		return ErrInvalidPassword
	}
	return nil
}
