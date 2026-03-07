package generator

import (
	"strings"
)

// --- Document Primitives ---

type Doc interface {
	isDoc()
}

type docText struct{ s string }
type docLine struct{}
type docNest struct {
	depth int
	doc   Doc
}
type docConcat struct{ docs []Doc }

// docGroup attempts to render its contents on a single line.
// If it exceeds the target width, docLine primitives turn into newlines.
type docGroup struct{ doc Doc }

func (docText) isDoc()   {}
func (docLine) isDoc()   {}
func (docNest) isDoc()   {}
func (docConcat) isDoc() {}
func (docGroup) isDoc()  {}

// --- Builder Functions ---

func doctext(s string) Doc        { return docText{s} }
func line() Doc                   { return docLine{} }
func nest(depth int, doc Doc) Doc { return docNest{depth, doc} }
func group(doc Doc) Doc           { return docGroup{doc} }
func concat(docs ...Doc) Doc      { return docConcat{docs: docs} }
func space() Doc                  { return doctext(" ") }
func textSpace(s string) Doc      { return concat(doctext(s), space()) }

// --- The Layout Engine ---

type prettyPrinter struct {
	maxWidth int
}

func newPrettyPrinter(maxWidth int) *prettyPrinter {
	return &prettyPrinter{maxWidth: maxWidth}
}

func (p *prettyPrinter) render(doc Doc) string {
	var b strings.Builder
	p.format(&b, 0, false, doc)
	return b.String()
}

func (p *prettyPrinter) format(b *strings.Builder, currentIndent int, flatten bool, doc Doc) {
	switch d := doc.(type) {
	case docText:
		b.WriteString(d.s)
	case docLine:
		p.formatLine(b, currentIndent, flatten)
	case docNest:
		p.format(b, currentIndent+d.depth, flatten, d.doc)
	case docConcat:
		p.formatConcat(b, currentIndent, flatten, d)
	case docGroup:
		p.formatGroup(b, currentIndent, flatten, d)
	}
}

func (p *prettyPrinter) formatLine(b *strings.Builder, currentIndent int, flatten bool) {
	if flatten {
		b.WriteString(" ")
		return
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("\t", currentIndent))
}

func (p *prettyPrinter) formatConcat(b *strings.Builder, currentIndent int, flatten bool, d docConcat) {
	for _, child := range d.docs {
		p.format(b, currentIndent, flatten, child)
	}
}

func (p *prettyPrinter) formatGroup(b *strings.Builder, currentIndent int, flatten bool, d docGroup) {
	fits := p.fits(p.maxWidth, d.doc)
	p.format(b, currentIndent, flatten || fits, d.doc)
}

func (p *prettyPrinter) fits(width int, doc Doc) bool {
	if width < 0 {
		return false
	}
	// Simplified width calculation. A robust implementation requires
	// measuring the flattened width of the doc tree.
	return calculateFlatWidth(doc) <= width
}

func calculateFlatWidth(doc Doc) int {
	switch d := doc.(type) {
	case docText:
		return len(d.s)
	case docLine:
		return 1 // Space
	case docNest:
		return calculateFlatWidth(d.doc)
	case docConcat:
		w := 0
		for _, child := range d.docs {
			w += calculateFlatWidth(child)
		}
		return w
	case docGroup:
		return calculateFlatWidth(d.doc)
	}
	return 0
}
