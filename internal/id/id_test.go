package id

import (
	"strings"
	"testing"
)

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

func TestNewFormat(t *testing.T) {
	got := New()
	if len(got) != 26 {
		t.Fatalf("len(%q) = %d, want 26", got, len(got))
	}
	for _, r := range got {
		if !strings.ContainsRune(crockford, r) {
			t.Errorf("%q contains %q, which is not Crockford base32", got, r)
		}
	}
}

func TestNewUniqueAndIncreasing(t *testing.T) {
	prev := New()
	for i := range 1000 {
		next := New()
		if next <= prev {
			t.Fatalf("call %d: %q is not greater than %q", i, next, prev)
		}
		prev = next
	}
}
