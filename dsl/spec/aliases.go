package spec

import (
	"syntaxa"
	"syntaxa/rule"
)

type RuleBuilder = rule.RuleBuilder[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Rule = rule.Rule[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Result = rule.Result[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type Node = syntaxa.SyntaxaLSTNode[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type NodeFinalizationCtx = syntaxa.FinalizationCtx[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
