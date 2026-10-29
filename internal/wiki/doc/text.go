package doc

import "strings"

// extractText returns the plain text of a normalized document for search.
func extractText(root *node) string {
	var sb strings.Builder
	writeText(&sb, root)
	return strings.TrimSpace(sb.String())
}

func writeText(sb *strings.Builder, n *node) {
	switch n.Type {
	case "text":
		sb.WriteString(n.Text)
	case "hardBreak":
		sb.WriteByte(' ')
	case "mention":
		sb.WriteString(n.Attrs["label"].(string))
	case "inlineRoll":
		sb.WriteString(n.Attrs["formula"].(string))
	}
	for _, c := range n.Content {
		writeText(sb, c)
	}
	switch n.Type {
	case "paragraph", "heading", "codeBlock", "detailsSummary":
		sb.WriteByte('\n')
	}
}
