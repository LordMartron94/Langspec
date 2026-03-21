package dsl

import (
	"langspec/dsl/semantics"
	"langspec/dsl/spec"
)

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

type GrammarPackage = semantics.GrammarPackage
type SemanticEnv = semantics.SemanticEnv
