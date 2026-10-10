package server

import (
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// findElement returns the first <tag> element in n with attribute key=val.
func findElement(n *html.Node, tag, key, val string) *html.Node {
	if n.Type == html.ElementNode && n.Data == tag && htmlAttr(n, key) == val {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if e := findElement(c, tag, key, val); e != nil {
			return e
		}
	}
	return nil
}

// findElements returns all <tag> elements in n, in document order.
func findElements(n *html.Node, tag string) []*html.Node {
	var res []*html.Node
	if n.Type == html.ElementNode && n.Data == tag {
		res = append(res, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		res = append(res, findElements(c, tag)...)
	}
	return res
}

// closest returns n or its nearest ancestor that is a <tag> element, or nil.
func closest(n *html.Node, tag string) *html.Node {
	for ; n != nil; n = n.Parent {
		if n.Type == html.ElementNode && n.Data == tag {
			return n
		}
	}
	return nil
}

// textContent returns the text of n and all its descendants, trimmed.
func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

// rowCells returns the texts of row's cells keyed by their column header in
// the row's table. Only use for tables with a single header row.
func rowCells(row *html.Node) map[string]string {
	headers := findElements(closest(row, "table"), "th")
	cells := findElements(row, "td")
	res := make(map[string]string, len(cells))
	for i, cell := range cells {
		if i < len(headers) {
			res[textContent(headers[i])] = textContent(cell)
		}
	}
	return res
}

func hasAttr(n *html.Node, key string) bool {
	return slices.ContainsFunc(n.Attr, func(a html.Attribute) bool { return a.Key == key })
}

func htmlAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
