package dsl

import (
	"syntaxa"
)

/*
LangSpec grammar registry: canonical mapping from LangSpecParserNodeKind to syntaxa.GrammarID
for nodes that produce an AST slot. Used by GrammarDefiner to resolve GrammarID from NodeKind
so callers can pass only node and token. Virtual expectations still pass GrammarID explicitly.
*/

var langSpecNodeToGrammarID = map[LangSpecParserNodeKind]syntaxa.GrammarID{
	NodeDSLName:         GrammarIDDSLName,
	NodeVersion:         GrammarIDDSLVersion,
	NodeLSPECName:       GrammarIDLangspecName,
	NodePragmaKey:       GrammarIDPragmaKey,
	NodePragmaValue:     GrammarIDPragmaValue,
	NodeMetaKey:         GrammarIDMetaKey,
	NodeMetaValue:       GrammarIDMetaValue,
	NodeLexKeyword:      GrammarIDLexKeyword,
	NodeLexRuleTokenName: GrammarIDLexRuleTokenName,
	NodeLexRuleScope:    GrammarIDLexRuleScope,
	NodeLexRulePattern:  GrammarIDLexRulePattern,
	NodeLexRulePriority: GrammarIDLexRulePriority,
}

/*
LangSpecGrammarIDForNode returns the canonical GrammarID for the given node kind, if any.
Used by GrammarDefiner for ExpectToken and ExpectOneOf. Returns false for node kinds
that have no single canonical grammar ID (e.g. structural or context-dependent).
*/
func LangSpecGrammarIDForNode(node LangSpecParserNodeKind) (syntaxa.GrammarID, bool) {
	id, ok := langSpecNodeToGrammarID[node]
	return id, ok
}
