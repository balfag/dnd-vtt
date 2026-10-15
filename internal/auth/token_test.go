package auth

import (
	"encoding/base64"
	"testing"
)

func TestNewToken(t *testing.T) {
	a, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(a) != 43 {
		t.Errorf("len = %d, want 43", len(a))
	}
	raw, err := base64.RawURLEncoding.DecodeString(a)
	if err != nil || len(raw) != 32 {
		t.Errorf("decode = %d bytes, %v; want 32 bytes", len(raw), err)
	}
	if a == b {
		t.Error("two tokens are equal")
	}
}

func TestHashToken(t *testing.T) {
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got := HashToken("abc"); got != want {
		t.Errorf("HashToken(abc) = %s, want %s", got, want)
	}
}
