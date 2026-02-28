package dsl

import (
	"autarch/pattern"
	"syntaxa"
)

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

// ----------------------------------------------------------- TOKEN BUILDER

/*
TokenBuilder configures a single token via a fluent API.
DefineToken starts the chain; Build returns the TokenDefinition.
*/
type TokenBuilder struct {
	def TokenDefinition
}

/*
DefineToken starts a fluent token definition with sensible defaults:
Role LANG_SPEC_STRUCTURAL_ROLE, Priority 0, Pattern nil.
*/
func DefineToken(t LangSpecLexerTokenType) *TokenBuilder {
	return &TokenBuilder{
		def: TokenDefinition{
			Type:     t,
			Role:     LANG_SPEC_STRUCTURAL_ROLE,
			Priority: 0,
		},
	}
}

func (b *TokenBuilder) Role(r LangSpecLexerTokenRole) *TokenBuilder {
	b.def.Role = r
	return b
}

func (b *TokenBuilder) Scope(s string) *TokenBuilder {
	b.def.Scope = s
	return b
}

func (b *TokenBuilder) HighPriority() *TokenBuilder {
	b.def.Priority = 1
	return b
}

func (b *TokenBuilder) Priority(priority int) *TokenBuilder {
	b.def.Priority = priority
	return b
}

func (b *TokenBuilder) Pattern(p pattern.RegulaAST[rune]) *TokenBuilder {
	q := new(pattern.RegulaAST[rune])
	*q = p
	b.def.Pattern = q
	return b
}

func (b *TokenBuilder) Build() TokenDefinition {
	return b.def
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

// ----------------------------------------------------------- EXPECT BUILDER

/*
ExpectBuilder configures a single header expectation via a fluent API.
Expect starts the chain; Build returns the DSLHeaderExpectation.
*/
type ExpectBuilder struct {
	expect DSLHeaderExpectation
}

/*
Expect starts a fluent header expectation with sensible defaults:
NestAction DSLNestActionMatch; other fields zero.
*/
func Expect(id syntaxa.GrammarID) *ExpectBuilder {
	return &ExpectBuilder{
		expect: DSLHeaderExpectation{
			GrammarID:  id,
			NestAction: DSLNestActionMatch,
		},
	}
}

func (b *ExpectBuilder) Node(kind LangSpecParserNodeKind) *ExpectBuilder {
	b.expect.NodeKind = kind
	return b
}

func (b *ExpectBuilder) Tokens(tokens ...LangSpecLexerTokenType) *ExpectBuilder {
	b.expect.Tokens = tokens
	return b
}

func (b *ExpectBuilder) Virtual() *ExpectBuilder {
	b.expect.Virtual = true
	return b
}

func (b *ExpectBuilder) PushNext() *ExpectBuilder {
	b.expect.NestAction = DSLNestActionPushNext
	return b
}

func (b *ExpectBuilder) Pop(count int) *ExpectBuilder {
	b.expect.NestAction = DSLNestActionPop
	b.expect.PopCount = count
	return b
}

func (b *ExpectBuilder) ScopeOverride(scope string) *ExpectBuilder {
	b.expect.ScopeOverride = scope
	return b
}

func (b *ExpectBuilder) NestOnly() *ExpectBuilder {
	b.expect.NestOnly = true
	return b
}

func (b *ExpectBuilder) Build() DSLHeaderExpectation {
	return b.expect
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

func defHeaderStep(labelSuffix string, metaScope string, expectations ...DSLHeaderExpectation) DSLHeaderNestStep {
	return DSLHeaderNestStep{
		LabelSuffix:  labelSuffix,
		MetaScope:    metaScope,
		Expectations: expectations,
	}
}
