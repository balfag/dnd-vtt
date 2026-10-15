package web_test

import (
	"io/fs"
	"testing"

	"github.com/balfag/dnd-vtt/web"
)

func TestDistContainsPlaceholder(t *testing.T) {
	if _, err := fs.Stat(web.Dist(), ".gitkeep"); err != nil {
		t.Fatalf("Dist() has no .gitkeep: %v", err)
	}
}
