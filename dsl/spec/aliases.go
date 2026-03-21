package spec

import (
	"syntaxa"
	"syntaxa/rule"
)

/*
RuleBuilder, Rule, and Result are syntaxa rule types parameterized by LangSpec lexer/parser enums.
*/
type RuleBuilder = rule.RuleBuilder[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Rule = rule.Rule[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Result = rule.Result[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

/* Node is the LangSpec LST node type (rune stream, LangSpec token kinds and parse kinds). */
type Node = syntaxa.SyntaxaLSTNode[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

/* NodeFinalizationCtx is the finalization context passed when lowering LangSpec parse rules. */
type NodeFinalizationCtx = syntaxa.FinalizationCtx[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
