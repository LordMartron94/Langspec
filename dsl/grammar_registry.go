package dsl

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"syntaxa"
)

/*
pascalCaseToSpaceUppercase converts a PascalCase identifier to space-separated uppercase
(e.g. "DSLName" -> "DSL NAME"). Used to derive a stable GrammarID from a node kind name.
*/
func pascalCaseToSpaceUppercase(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	var prevLower bool
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		isUpper := unicode.IsUpper(r)
		if isUpper && (b.Len() > 0 && prevLower) {
			b.WriteByte(' ')
		}
		b.WriteRune(unicode.ToUpper(r))
		prevLower = unicode.IsLower(r)
	}
	return b.String()
}

/*
LangSpecGrammarIDFromNode returns a stable syntaxa.GrammarID for the given node kind.
If suffix is non-empty, the result is derivedBase + " " + suffix (e.g. NodeHeader + "CONTENT" -> "HEADER CONTENT").
NodeLSPECName is special-cased to "LANGSPEC NAME" for display consistency.
NodeMetaKeyValuePair + suffix "SEQUENCE" is special-cased to "META KEY VALUE SEQUENCE".
*/
func LangSpecGrammarIDFromNode(node LangSpecParserNodeKind, suffix string) syntaxa.GrammarID {
	if node == NodeMetaKeyValuePair && suffix == "SEQUENCE" {
		return "META KEY VALUE SEQUENCE"
	}
	name := node.String()
	name = strings.TrimPrefix(name, "Node")
	if name == "LSPECName" {
		name = "LANGSPEC NAME"
	} else {
		name = pascalCaseToSpaceUppercase(name)
	}
	if suffix != "" {
		return syntaxa.GrammarID(name + " " + suffix)
	}
	return syntaxa.GrammarID(name)
}

/*
VirtualGrammarID identifies a grammar rule that has no AST node (e.g. EOF, punctuation-only slots).
Used for virtual expectations; the string form is produced by VirtualGrammarIDToGrammarID.
*/
type VirtualGrammarID uint8

const (
	VirtualEOF VirtualGrammarID = iota + 1
	VirtualHeaderDashes
	VirtualHeaderSeparator
	VirtualMetaAssignment
)

/*
VirtualGrammarIDToGrammarID returns the canonical syntaxa.GrammarID string for the virtual ID.
Stable and used by GrammarDefiner when building virtual expectations.
*/
func VirtualGrammarIDToGrammarID(v VirtualGrammarID) syntaxa.GrammarID {
	switch v {
	case VirtualEOF:
		return "EOF"
	case VirtualHeaderDashes:
		return "HEADER DASHES"
	case VirtualHeaderSeparator:
		return "HEADER SEPARATOR"
	case VirtualMetaAssignment:
		return "META ASSIGNMENT"
	default:
		return syntaxa.GrammarID("VIRTUAL_UNKNOWN")
	}
}
