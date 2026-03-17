package editor

import (
	"autarch/pattern"
)

/*
TextPatternBuilder builds OverrideHandler values for common editor patterns.

Use with OverrideRegistry: create a builder with NewTextPatternBuilder (pass a rune
RegulaASTFactory), then call LineComment or BlockComment to obtain handlers and
Register them for the appropriate tokens. These default patterns replace the
single-rule transition with capture/delimited behavior expected by editors.
*/
type TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	factory *pattern.RegulaASTFactory[rune]
}

/*
NewTextPatternBuilder creates a TextPatternBuilder that uses the given factory for
pattern construction. The factory domain must be rune for text-based languages.
*/
func NewTextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	factory *pattern.RegulaASTFactory[rune],
) *TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		factory: factory,
	}
}

/*
LineComment returns an OverrideHandler that implements line-comment highlighting.

prefix is the comment start (e.g. "//"). matchContext is applied to the whole comment;
punctuationContext is applied to the first capture (the prefix). The pattern matches
prefix then any runes until newline or carriage return. Register the returned handler
for your line-comment token in an OverrideRegistry.
*/
func (b *TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext]) LineComment(
	prefix string,
	matchContext TContext,
	punctuationContext TContext,
) OverrideHandler[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return func(_ *EditorCtx[rune, TToken, TTokenRole, TLexerState, TNodeKind]) []*EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
		prefixPattern := pattern.LiteralString(b.factory, prefix).Capture()
		notTerminator := b.factory.NegatedClass(
			b.factory.Range('\n', '\n'),
			b.factory.Range('\r', '\r'),
		).Star().Capture()

		newPattern := prefixPattern.Then(notTerminator)

		return []*EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{{
			Pattern:      &newPattern,
			MatchContext: &matchContext,
			Captures: map[int]TContext{
				1: punctuationContext,
			},
		}}
	}
}

/*
BlockComment returns an OverrideHandler that implements block-comment highlighting.

open and close are the literal delimiters (e.g. block-comment start and end strings).
bodyMetaContext is the context for the region between them; openContext and closeContext
are applied to the open and close tokens. The handler uses DelimitedPayload so the IR
emits a nested state. Register the returned handler for your block-comment token in an
OverrideRegistry.
*/
func (b *TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext]) BlockComment(
	open string,
	close string,
	bodyMetaContext TContext,
	openContext TContext,
	closeContext TContext,
) OverrideHandler[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return func(_ *EditorCtx[rune, TToken, TTokenRole, TLexerState, TNodeKind]) []*EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
		openPattern := pattern.LiteralString(b.factory, open)
		closePattern := pattern.LiteralString(b.factory, close)

		return []*EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{{
			Pattern:      &openPattern,
			MatchContext: &openContext,
			DelimitedPayload: &DelimitedPayload[rune, TContext]{
				StateLabel:   "block_comment_inner",
				BodyContext:  bodyMetaContext,
				ClosePattern: closePattern,
				CloseContext: closeContext,
			},
		}}
	}
}
