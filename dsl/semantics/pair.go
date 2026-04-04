package semantics

import (
	"strings"

	. "langspec/dsl/spec"
)

/*
PairDecl records one `pair` declaration in the PARSE section: logical name and open/close lexer
token names. Node is the NodeParsePair root for diagnostics.
*/
type PairDecl struct {
	Node       *Node
	OpenToken  string
	CloseToken string
}

/*
PairNameFromTokPairReferenceRaw returns the pair name from a TokPairReference lexeme body (leading
`@` stripped). Empty if invalid.
*/
func PairNameFromTokPairReferenceRaw(raw []byte) string {
	s := strings.TrimSpace(string(raw))
	if len(s) < 2 || s[0] != '@' {
		return ""
	}
	name := strings.TrimSpace(s[1:])
	if name == "" {
		return ""
	}
	return name
}

/*
PairNameFromNestPairRefNode returns the pair name for a NodeParseNestPairRef node, or "".
*/
func PairNameFromNestPairRefNode(n *Node) string {
	if n == nil || n.Kind() != NodeParseNestPairRef {
		return ""
	}
	toks := n.Tokens()
	if len(toks) == 0 {
		return ""
	}
	return PairNameFromTokPairReferenceRaw(toks[0].Raw)
}

/*
ExtractPairDeclaration returns the pair name and open/close token names from a NodeParsePair node.
ok is false only if the LST is incomplete (e.g. partial parse); a successful parse of the LangSpec
grammar guarantees two token references after the pair identifier.
*/
func ExtractPairDeclaration(pairNode *Node) (pairName, openTok, closeTok string, ok bool) {
	if pairNode == nil || pairNode.Kind() != NodeParsePair {
		return "", "", "", false
	}
	idNode := pairNode.FindFirstKind(NodeParsePairIdentifier)
	if idNode == nil {
		return "", "", "", false
	}
	pairName = IdentifierValue(idNode)
	if pairName == "" {
		return "", "", "", false
	}
	refs := pairNode.FindAllKind(NodeParseTokenReference)
	if len(refs) < 2 {
		return pairName, "", "", false
	}
	openTok = IdentifierValue(refs[0])
	closeTok = IdentifierValue(refs[1])
	if openTok == "" || closeTok == "" {
		return pairName, openTok, closeTok, false
	}
	return pairName, openTok, closeTok, true
}
