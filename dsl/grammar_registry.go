package dsl

import (
	"syntaxa"
)

const (
	// Root & General
	GrammarIDProgram    syntaxa.GrammarID = "PROGRAM"
	GrammarIDEOF        syntaxa.GrammarID = "EOF"
	GrammarIDBlockClose syntaxa.GrammarID = "BLOCK CLOSE"

	GrammarIDVariableReference   syntaxa.GrammarID = "VARIABLE REFERENCE"
	GrammarIDRange               syntaxa.GrammarID = "RANGE"
	GrammarIDCharacterRangeStart syntaxa.GrammarID = "RANGE START"
	GrammarIDCharacterRangeEnd   syntaxa.GrammarID = "RANGE END"
	GrammarIDCharacterLiteral    syntaxa.GrammarID = "CHARACTER LITERAL"

	// Header
	GrammarIDHeader          syntaxa.GrammarID = "HEADER"
	GrammarIDHeaderContent   syntaxa.GrammarID = "HEADER CONTENT"
	GrammarIDHeaderSeparator syntaxa.GrammarID = "HEADER SEPARATOR"
	GrammarIDHeaderDashes    syntaxa.GrammarID = "HEADER DASHES"
	GrammarIDDSLName         syntaxa.GrammarID = "DSL NAME"
	GrammarIDDSLVersion      syntaxa.GrammarID = "DSL VERSION"
	GrammarIDLangspecName    syntaxa.GrammarID = "LANGSPEC NAME"
	GrammarIDLangspecVersion syntaxa.GrammarID = "LANGSPEC VERSION"

	// Pragma Section
	GrammarIDPragmaSection       syntaxa.GrammarID = "PRAGMA SECTION"
	GrammarIDPragmaStatement     syntaxa.GrammarID = "PRAGMA STATEMENT"
	GrammarIDPragmaStatementBody syntaxa.GrammarID = "PRAGMA STATEMENT BODY"
	GrammarIDPragmaStart         syntaxa.GrammarID = "PRAGMA START"
	GrammarIDPragmaEnd           syntaxa.GrammarID = "PRAGMA END"

	GrammarIDPragmaKey   syntaxa.GrammarID = "PRAGMA KEY"
	GrammarIDPragmaValue syntaxa.GrammarID = "PRAGMA VALUE"

	// Meta Section
	GrammarIDMetaSection      syntaxa.GrammarID = "META SECTION"
	GrammarIDMetaKeyValuePair syntaxa.GrammarID = "META KEY VALUE PAIR"
	GrammarIDMetaKeyValueSeq  syntaxa.GrammarID = "META KEY VALUE SEQUENCE"

	GrammarIDMetaKey        syntaxa.GrammarID = "META KEY"
	GrammarIDMetaValue      syntaxa.GrammarID = "META VALUE"
	GrammarIDMetaAssignment syntaxa.GrammarID = "META ASSIGNMENT"

	// Lex Section
	GrammarIDLexSection       syntaxa.GrammarID = "LEX SECTION"
	GrammarIDLexSectionBody   syntaxa.GrammarID = "LEX SECTION BODY"
	GrammarIDLexRuleList      syntaxa.GrammarID = "LEX RULE LIST"
	GrammarIDLexKeyword       syntaxa.GrammarID = "LEX KEYWORD"
	GrammarIDLexRule          syntaxa.GrammarID = "LEX RULE"
	GrammarIDLexRuleTokenName syntaxa.GrammarID = "LEX RULE TOKEN NAME"
	GrammarIDLexRuleScope     syntaxa.GrammarID = "LEX RULE SCOPE"
	GrammarIDLexRulePattern   syntaxa.GrammarID = "LEX RULE PATTERN"
	GrammarIDLexRulePriority  syntaxa.GrammarID = "LEX RULE PRIORITY"
)

/*
LangSpec grammar registry: canonical mapping from LangSpecParserNodeKind to syntaxa.GrammarID
for nodes that produce an AST slot. Used by GrammarDefiner to resolve GrammarID from NodeKind
so callers can pass only node and token. Virtual expectations still pass GrammarID explicitly.
*/

var langSpecNodeToGrammarID = map[LangSpecParserNodeKind]syntaxa.GrammarID{
	NodeDSLName:          GrammarIDDSLName,
	NodeVersion:          GrammarIDDSLVersion,
	NodeLSPECName:        GrammarIDLangspecName,
	NodePragmaKey:        GrammarIDPragmaKey,
	NodePragmaValue:      GrammarIDPragmaValue,
	NodeMetaKey:          GrammarIDMetaKey,
	NodeMetaValue:        GrammarIDMetaValue,
	NodeLexKeyword:       GrammarIDLexKeyword,
	NodeLexRuleTokenName: GrammarIDLexRuleTokenName,
	NodeLexRuleScope:     GrammarIDLexRuleScope,
	NodeLexRulePattern:   GrammarIDLexRulePattern,
	NodeLexRulePriority:  GrammarIDLexRulePriority,
}

/*
langSpecGrammarIDForNode returns the canonical GrammarID for the given node kind, if any.
Used by GrammarDefiner for ExpectToken and ExpectOneOf. Returns false for node kinds
that have no single canonical grammar ID (e.g. structural or context-dependent).
*/
func langSpecGrammarIDForNode(node LangSpecParserNodeKind) (syntaxa.GrammarID, bool) {
	id, ok := langSpecNodeToGrammarID[node]
	return id, ok
}
