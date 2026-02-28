package dsl

import (
	"autarch/pattern"
	"syntaxa"
)

// ----------------------------------------------------------- LEXER / PARSER ENUMS (single place for definition)

//go:generate stringer -type LangSpecLexerState
type LangSpecLexerState uint8

const (
	LANG_SPEC_LEXER_STATE_DEFAULT LangSpecLexerState = iota + 1
)

//go:generate stringer -type LangSpecLexerTokenType
type LangSpecLexerTokenType uint32

const (
	TokEOF LangSpecLexerTokenType = iota + 1
	TokWhitespace
	TokDashes
	TokHeaderSeparator
	TokStringLiteral
	TokVersion
	TokKWLSpec
	TokBraceOpen
	TokBraceClose
	TokSemicolon
	TokComma
	TokLineComment
	TokBlockComment
)

//go:generate stringer -type LangSpecLexerTokenRole
type LangSpecLexerTokenRole uint8

const (
	LANG_SPEC_STRUCTURAL_ROLE LangSpecLexerTokenRole = iota + 1
	LANG_SPEC_WHITESPACE_ROLE
	LANG_SPEC_COMMENT_ROLE
)

//go:generate stringer -type LangSpecParserNodeKind
type LangSpecParserNodeKind uint32

const (
	NodeError LangSpecParserNodeKind = iota + 1
	NodeProgram
	NodeHeader
	NodeHeaderContent
	NodeBody
	NodeDSLName
	NodeVersion
	NodeLSPECName
	NodeIdentifier
)

// ----------------------------------------------------------- TOKEN REGISTRY ENUMS

/*
TokenDefinition is the SSoT entry for one token: identity, role, scope, and how it matches.
Pattern is nil for virtual tokens (e.g. TokEOF). The lexer engine adds a rule only when Pattern != nil.
*/
type TokenDefinition struct {
	Type     LangSpecLexerTokenType
	Role     LangSpecLexerTokenRole
	Scope    string
	Priority int
	Pattern  *pattern.RegulaAST[rune]
}

/*
LanguageSpec is the single source of truth for the DSL: all token definitions and header steps.
Built by BuildLanguageSpec; consumed by BuildLexerSpec and BuildParserSpec.
*/
type LanguageSpec struct {
	Tokens  []TokenDefinition
	Headers []DSLHeaderNestStep
}

/*
LanguageSpecScopeMap returns a map from token type to scope string from the spec's tokens.
*/
func LanguageSpecScopeMap(spec LanguageSpec) map[LangSpecLexerTokenType]string {
	out := make(map[LangSpecLexerTokenType]string, len(spec.Tokens))
	for _, t := range spec.Tokens {
		if t.Scope != "" {
			out[t.Type] = t.Scope
		}
	}
	return out
}

/*
LanguageSpecHeaderExpectationsFlat returns the header expectations in parser order,
excluding NestOnly entries.
*/
func LanguageSpecHeaderExpectationsFlat(spec LanguageSpec) []DSLHeaderExpectation {
	var out []DSLHeaderExpectation
	for _, step := range spec.Headers {
		for _, e := range step.Expectations {
			if !e.NestOnly {
				out = append(out, e)
			}
		}
	}
	return out
}

// ----------------------------------------------------------- HEADER SPEC ENUMS

//go:generate stringer -type DSLNestAction
type DSLNestAction uint8

const (
	DSLNestActionMatch DSLNestAction = iota
	DSLNestActionPushNext
	DSLNestActionPop
)

/*
DSLHeaderExpectation is one slot in the header content: grammar ID, node kind, token(s),
and nest action. ScopeOverride and NestOnly are optional.
*/
type DSLHeaderExpectation struct {
	GrammarID     syntaxa.GrammarID
	NodeKind      LangSpecParserNodeKind
	Tokens        []LangSpecLexerTokenType
	Virtual       bool
	NestAction    DSLNestAction
	PopCount      int
	ScopeOverride string
	NestOnly      bool
}

/*
DSLHeaderNestStep is one state in the header nest (e.g. expect_name, expect_version, expect_tail).
*/
type DSLHeaderNestStep struct {
	LabelSuffix  string
	MetaScope    string
	Expectations []DSLHeaderExpectation
}

// ----------------------------------------------------------- HEADER SPEC FACTORIES

func defHeaderExpect(grammarID syntaxa.GrammarID, nodeKind LangSpecParserNodeKind, tokens []LangSpecLexerTokenType, virtual bool, action DSLNestAction, popCount int, scopeOverride string, nestOnly bool) DSLHeaderExpectation {
	return DSLHeaderExpectation{
		GrammarID:     grammarID,
		NodeKind:      nodeKind,
		Tokens:        tokens,
		Virtual:       virtual,
		NestAction:    action,
		PopCount:      popCount,
		ScopeOverride: scopeOverride,
		NestOnly:      nestOnly,
	}
}

func defHeaderStep(labelSuffix string, metaScope string, expectations ...DSLHeaderExpectation) DSLHeaderNestStep {
	return DSLHeaderNestStep{
		LabelSuffix:  labelSuffix,
		MetaScope:    metaScope,
		Expectations: expectations,
	}
}
