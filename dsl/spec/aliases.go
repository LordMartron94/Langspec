package spec

import (
	"syntaxa"
	"syntaxa/rule"
)

/*
RuleBuilder, Rule, and Result are syntaxa rule types parameterized by LangSpec lexer/parser enums.
*/
type RuleBuilder = rule.RuleBuilder[LangSpecParserNodeKind]
type Rule = rule.Rule[LangSpecParserNodeKind]
type Result = rule.Result[LangSpecParserNodeKind]

/* Node is the LangSpec LST node type (rune stream, LangSpec token kinds and parse kinds). */
type Node = syntaxa.SyntaxaLSTNode[LangSpecParserNodeKind]

/* NodeFinalizationCtx is the finalization context passed when lowering LangSpec parse rules. */
type NodeFinalizationCtx = syntaxa.FinalizationCtx[LangSpecParserNodeKind]
