package editor

import "cmp"

/*
OverrideHandler is a function that, given an EditorCtx for a token, returns an EditorOverride.

Use with OverrideRegistry: register a handler per token so the registry can produce overrides
when building EditorIR (e.g. line comment with capture, block comment delimited region).
*/
type OverrideHandler[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] func(ctx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]

/*
OverrideRegistry maps tokens to override handlers so a single override producer can be built.

Create with NewOverrideRegistry, register handlers with Register (e.g. from TextPatternBuilder),
then pass Producer() as the overrideProducer in EditorIRConfigurationCreate. When building
EditorIR, the engine calls the producer for each token; the producer looks up the token in the
registry and returns the handler's EditorOverride when registered.
*/
type OverrideRegistry[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	handlers map[TToken]OverrideHandler[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
}

/*
NewOverrideRegistry allocates an empty OverrideRegistry.

Register token handlers then use Producer() as the override producer in EditorIRConfiguration.
*/
func NewOverrideRegistry[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any]() *OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		handlers: make(map[TToken]OverrideHandler[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]),
	}
}

/*
Register associates a token with an override handler.

Later, Producer() will call this handler when the editor IR builder asks for an override
for that token. Registering the same token again overwrites the previous handler.
*/
func (r *OverrideRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) Register(
	token TToken,
	handler OverrideHandler[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) {
	r.handlers[token] = handler
}

/*
Producer returns a function suitable for EditorIRConfiguration.overrideProducer.

The returned function looks up the context's token in the registry; if a handler is
registered, it is invoked and the result is returned with hasOverride true. Otherwise
returns (nil, false). When ctx.Token is nil, returns (nil, false).
*/
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
