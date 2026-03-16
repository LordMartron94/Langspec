package editor

import (
	"autarch/pattern"
)

type TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	factory *pattern.RegulaASTFactory[rune]
}

func NewTextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	factory *pattern.RegulaASTFactory[rune],
) *TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		factory: factory,
	}
}

func (b *TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext]) LineComment(
	prefix string,
	matchContext TContext,
	punctuationContext TContext,
) OverrideHandler[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return func(_ *EditorCtx[rune, TToken, TTokenRole, TLexerState, TNodeKind]) *EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
		prefixPattern := pattern.LiteralString(b.factory, prefix).Capture()
		notTerminator := b.factory.NegatedClass(
			b.factory.Range('\n', '\n'),
			b.factory.Range('\r', '\r'),
		).Star().Capture()

		newPattern := prefixPattern.Then(notTerminator)

		return &EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
			Pattern:      &newPattern,
			MatchContext: &matchContext,
			Captures: map[int]TContext{
				1: punctuationContext,
			},
		}
	}
}

func (b *TextPatternBuilder[TToken, TTokenRole, TLexerState, TNodeKind, TContext]) BlockComment(
	open string,
	close string,
	bodyMetaContext TContext,
	openContext TContext,
	closeContext TContext,
) OverrideHandler[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return func(_ *EditorCtx[rune, TToken, TTokenRole, TLexerState, TNodeKind]) *EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
		openPattern := pattern.LiteralString(b.factory, open)
		closePattern := pattern.LiteralString(b.factory, close)

		return &EditorOverride[rune, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
			Pattern:      &openPattern,
			MatchContext: &openContext,
			DelimitedPayload: &DelimitedPayload[rune, TContext]{
				StateLabel:   "block_comment_inner",
				BodyContext:  bodyMetaContext,
				ClosePattern: closePattern,
				CloseContext: closeContext,
			},
		}
	}
}
