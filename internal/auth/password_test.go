package auth

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
)

var phcPattern = regexp.MustCompile(`^\$argon2id\$v=19\$m=65536,t=1,p=4\$[A-Za-z0-9+/]{22}\$[A-Za-z0-9+/]{43}$`)

func TestHashPasswordFormat(t *testing.T) {
	h1, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if !phcPattern.MatchString(h1) {
		t.Errorf("hash %q does not match the PHC format", h1)
	}
	h2, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Error("two hashes of the same password are equal, the salt is not random")
	}
}

func TestVerifyPassword(t *testing.T) {
	h, err := HashPassword("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := VerifyPassword(h, "correct horse"); err != nil || !ok {
		t.Errorf("right password: ok = %v, err = %v; want true, nil", ok, err)
	}
	if ok, err := VerifyPassword(h, "wrong horse"); err != nil || ok {
		t.Errorf("wrong password: ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestVerifyPasswordReadsParamsFromHash(t *testing.T) {
	salt := []byte("0123456789abcdef")
	key := argon2.IDKey([]byte("secret pass"), salt, 2, 1024, 1, 32)
	enc := fmt.Sprintf("$argon2id$v=19$m=1024,t=2,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))

	if ok, err := VerifyPassword(enc, "secret pass"); err != nil || !ok {
		t.Errorf("ok = %v, err = %v; want true, nil", ok, err)
	}
	if ok, err := VerifyPassword(enc, "other pass"); err != nil || ok {
		t.Errorf("ok = %v, err = %v; want false, nil", ok, err)
	}
}

func TestVerifyPasswordMalformed(t *testing.T) {
	tests := []struct{ name, enc string }{
		{"empty", ""},
		{"wrong algorithm", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$a2V5"},
		{"wrong version", "$argon2id$v=18$m=1,t=1,p=1$c2FsdA$a2V5"},
		{"missing p", "$argon2id$v=19$m=1,t=1$c2FsdA$a2V5"},
		{"trailing junk in params", "$argon2id$v=19$m=1,t=1,p=1x$c2FsdA$a2V5"},
		{"zero memory", "$argon2id$v=19$m=0,t=1,p=1$c2FsdA$a2V5"},
		{"bad salt base64", "$argon2id$v=19$m=1,t=1,p=1$!!!$a2V5"},
		{"empty key", "$argon2id$v=19$m=1,t=1,p=1$c2FsdA$"},
		{"five parts", "$argon2id$v=19$m=1,t=1,p=1$c2FsdA"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok, err := VerifyPassword(tt.enc, "x")
			if err == nil || ok {
				t.Fatalf("ok = %v, err = %v; want false and an error", ok, err)
			}
			if !strings.Contains(err.Error(), "malformed password hash") {
				t.Errorf("err = %q, want it to mention malformed password hash", err)
			}
		})
	}
}

func TestNormalizeLogin(t *testing.T) {
	good := map[string]string{"Anna": "anna", "a.b-c_1": "a.b-c_1", "MASTER": "master"}
	for in, want := range good {
		got, err := NormalizeLogin(in)
		if err != nil || got != want {
			t.Errorf("NormalizeLogin(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"ab", strings.Repeat("a", 33), "bad login", "юзер", "a/b"} {
		if _, err := NormalizeLogin(in); !errors.Is(err, ErrInvalidLogin) {
			t.Errorf("NormalizeLogin(%q) err = %v, want ErrInvalidLogin", in, err)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name string
		pw   string
		ok   bool
	}{
		{"7 chars", strings.Repeat("a", 7), false},
		{"8 chars", strings.Repeat("a", 8), true},
		{"128 chars", strings.Repeat("a", 128), true},
		{"129 chars", strings.Repeat("a", 129), false},
		{"8 runes, 16 bytes", "пароль12", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.pw)
			if tt.ok && err != nil {
				t.Errorf("err = %v, want nil", err)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidPassword) {
				t.Errorf("err = %v, want ErrInvalidPassword", err)
			}
		})
	}
}
