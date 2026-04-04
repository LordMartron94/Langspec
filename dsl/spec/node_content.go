package spec

import (
	"strings"
)

/*
NodeSingleTokenContent returns the raw string content of the node's single token.
Panics if node is nil or the node does not have exactly one token.
*/
func NodeSingleTokenContent(node *Node) string {
	if node == nil {
		panic("engine error: extraction called on nil node")
	}
	tks := node.Tokens()
	if len(tks) != 1 {
		panic("engine error: single token content extraction requires node to have exactly 1 token")
	}
	return string(tks[0].Raw)
}

/*
IdentifierValue returns the raw string content of the node's single token.
Panics if node does not have exactly one token. Use for identifier-like nodes.
*/
func IdentifierValue(node *Node) string {
	return NodeSingleTokenContent(node)
}

/*
TrimmedIdentifierNodeContent returns the trimmed raw content of the node's first token,
or empty string if node is nil or has no tokens. Use when the node may be absent.
*/
func TrimmedIdentifierNodeContent(node *Node) string {
	if node == nil || len(node.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(NodeSingleTokenContent(node))
}

/*
RefName returns the trimmed identifier content of a reference node, or empty string if nil or no tokens.
*/
func RefName(node *Node) string {
	if node == nil || len(node.Tokens()) == 0 {
		return ""
	}

	kind := node.Kind()
	if kind == NodeParseExpressionReference || kind == NodeParseTokenReference || kind == NodeParseSymbolReference ||
		kind == NodeParseTemplateReference {
		return strings.TrimSpace(IdentifierValue(node))
	}

	if kind == NodeParseNodeName {
		parent := node.Parent()
		if parent != nil && parent.Kind() == NodeParseSegment {
			hasTokenTail := parent.FindFirstKind(NodeParseTokenReference) != nil
			hasGroupTail := parent.FindFirstKind(NodeParseGroup) != nil

			if !hasTokenTail && !hasGroupTail {
				return strings.TrimSpace(IdentifierValue(node))
			}
		}
	}

	return ""
}

/*
ParseRuleName returns the trimmed name from a parse rule name node.
*/
func ParseRuleName(nameNode *Node) string {
	return TrimmedIdentifierNodeContent(nameNode)
}

/*
ExtractPatternDefName returns the pattern definition name and the name node, or ("", nil) if absent.
*/
func ExtractPatternDefName(def *Node) (string, *Node) {
	nameNode := def.FindFirstKind(NodePatternDefName)
	if nameNode == nil || len(nameNode.Tokens()) == 0 {
		return "", nil
	}
	return string(nameNode.Tokens()[0].Raw), nameNode
}
