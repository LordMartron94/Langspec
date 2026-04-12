package dsl

import (
	"langspec/dsl/semantics"
	"langspec/dsl/spec"
)

const LangSpecLexerStateInitial = spec.LangSpecLexerStateInitial

/*
Re-exports spec token/node kind aliases and Node for the dsl package API.
*/
type (
	LangSpecLexerState     = spec.LangSpecLexerState
	LangSpecLexerTokenType = spec.LangSpecLexerTokenType
	LangSpecLexerTokenRole = spec.LangSpecLexerTokenRole
	LangSpecParserNodeKind = spec.LangSpecParserNodeKind
	RuleBuilder            = spec.RuleBuilder
	Rule                   = spec.Rule
	Result                 = spec.Result
	Node                   = spec.Node
	LanguageSpec           = spec.LanguageSpec
)

/* GrammarPackage is the lowered grammar package type (see semantics.GrammarPackage). */
type GrammarPackage[TNodeKind ~uint32] = semantics.GrammarPackage[TNodeKind]

/* SemanticEnv is the symbol environment built from the LST (see semantics.SemanticEnv). */
type SemanticEnv = semantics.SemanticEnv
