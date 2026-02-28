package dsl

import "syntaxa"

// Grammar and rule IDs used when building the LangSpec DSL grammar and when
// registering editor overrides. Centralized so parsing and the compiler stay
// consistent; renames and refactors touch one place only.

const (
	GrammarIDProgram            syntaxa.GrammarID = "PROGRAM"
	GrammarIDHeader             syntaxa.GrammarID = "HEADER"
	GrammarIDHeaderContent      syntaxa.GrammarID = "HEADER CONTENT"
	GrammarIDDeclarationBlocks  syntaxa.GrammarID = "DECLARATION BLOCKS"
	GrammarIDDeclarationBlock   syntaxa.GrammarID = "DECLARATION BLOCK"
	GrammarIDDeclareList        syntaxa.GrammarID = "DECLARE LIST"
	GrammarIDEOF                syntaxa.GrammarID = "EOF"
	GrammarIDHeaderSeparator    syntaxa.GrammarID = "HEADER SEPARATOR"
	GrammarIDDSLName            syntaxa.GrammarID = "DSL NAME"
	GrammarIDDSLVersion         syntaxa.GrammarID = "DSL VERSION"
	GrammarIDLangspecName       syntaxa.GrammarID = "LANGSPEC NAME"
	GrammarIDLangspecVersion    syntaxa.GrammarID = "LANGSPEC VERSION"
	GrammarIDDeclareKeyword     syntaxa.GrammarID = "DECLARE KEYWORD"
	GrammarIDDeclareIdentifier  syntaxa.GrammarID = "DECLARE IDENTIFIER"
	GrammarIDBlockClose         syntaxa.GrammarID = "BLOCK CLOSE"
)
