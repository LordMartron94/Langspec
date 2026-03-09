package dsl

import (
	"strings"
)

/*
nodeSingleTokenContent returns the raw string content of the node's single token.
Panics if node is nil or the node does not have exactly one token.
*/
func nodeSingleTokenContent(node *Node) string {
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
getIdentifierValue returns the raw string content of the node's single token.
Panics if node does not have exactly one token. Use for identifier-like nodes.
*/
func getIdentifierValue(node *Node) string {
	return nodeSingleTokenContent(node)
}

/*
getTrimmedIdentifierNodeContent returns the trimmed raw content of the node's first token,
or empty string if node is nil or has no tokens. Use when the node may be absent.
*/
func getTrimmedIdentifierNodeContent(node *Node) string {
	if node == nil || len(node.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(nodeSingleTokenContent(node))
}

/*
getRefName returns the trimmed identifier content of a reference node, or empty string if nil or no tokens.
*/
func getRefName(refNode *Node) string {
	if refNode == nil || len(refNode.Tokens()) == 0 {
		return ""
	}
	return strings.TrimSpace(getIdentifierValue(refNode))
}

/*
getParseRuleName returns the trimmed name from a parse rule name node.
*/
func getParseRuleName(nameNode *Node) string {
	return getTrimmedIdentifierNodeContent(nameNode)
}

/*
extractPatternDefName returns the pattern definition name and the name node, or ("", nil) if absent.
*/
func extractPatternDefName(def *Node) (string, *Node) {
	nameNode := def.FindFirstKind(NodePatternDefName)
	if nameNode == nil || len(nameNode.Tokens()) == 0 {
		return "", nil
	}
	return string(nameNode.Tokens()[0].Raw), nameNode
}
