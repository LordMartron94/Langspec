package editor

import "cmp"

type OverrideHandler[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] func(ctx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]

type OverrideRegistry[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	handlers map[TToken]OverrideHandler[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
}

func NewOverrideRegistry[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any]() *OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		handlers: make(map[TToken]OverrideHandler[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]),
	}
}

func (r *OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) Register(
	token TToken,
	handler OverrideHandler[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) {
	r.handlers[token] = handler
}

func (r *OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) Producer() func(ctx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) (*EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], bool) {
	return func(ctx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) (*EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], bool) {
		if ctx.Token == nil {
			return nil, false
		}

		handler, exists := r.handlers[*ctx.Token]
		if !exists {
			return nil, false
		}

		return handler(ctx), true
	}
}
