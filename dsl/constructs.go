package dsl

import (
	"autarch/pattern"
)

/*
TokenDefinition is the SSoT entry for one token: identity, role, scope, and how it matches.
Pattern is nil for virtual tokens (e.g. TokEOF). The lexer engine adds a rule only when Pattern != nil.
Open and Close are optional; when both are set, the token is a delimited region for editor IR.
*/
type TokenDefinition struct {
	Type     LangSpecLexerTokenType
	Role     LangSpecLexerTokenRole
	Scope    string
	Priority int
	Pattern  *pattern.RegulaAST[rune]
	Open     *pattern.RegulaAST[rune]
	Close    *pattern.RegulaAST[rune]
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

/*
Delimited sets the open and close patterns for a delimited region (e.g. block comment).
When both are set, the ruleset is annotated and the editor IR can map this token to
push/body/pop regions automatically.
*/
func (b *TokenBuilder) Delimited(open, close pattern.RegulaAST[rune]) *TokenBuilder {
	o := new(pattern.RegulaAST[rune])
	*o = open
	c := new(pattern.RegulaAST[rune])
	*c = close
	b.def.Open  = o
	b.def.Close = c
	return b
}

func (b *TokenBuilder) Build() TokenDefinition {
	return b.def
}

/*
LanguageSpec is the single source of truth for the DSL: all token definitions.
Built by BuildLanguageSpec; consumed by BuildLexerSpec and BuildParserSpec.
*/
type LanguageSpec struct {
	Tokens []TokenDefinition
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
