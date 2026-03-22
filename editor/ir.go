package editor

import (
	"autarch/pattern"
	"cmp"
	"foundation/hash"
	"lexarch"
	"syntaxa"
	"syntaxa/lowering"
)

/* ROOT_LABEL is the label for the synthetic root state in EditorIR. */
const ROOT_LABEL = "root"

/*
EditorCtx is the context passed to configuration callbacks when building EditorIR.

Token, TokenRole, and NodeKind describe the current transition; they may be nil when
not applicable. IsNest and NestLabel are set for a nest wrapper state; NodeKind may
also be set to the opening production kind (from lowering.StateGraph.NestContentParentNodeKind
on the sibling *_content id) so a manifest NodeBinding MetaScope replaces the default
nest ".body" meta for that wrapper.
IsInvalidContext is true when the context is being produced for the synthesised
catch-all/fallback transition of a state. Clients should return the
"invalid.illegal.unexpected-token" scope (or equivalent) when this flag is set.
Recovery transitions themselves are normal transitions and must NOT have
IsInvalidContext set; only the catch-all fallback does.
*/
type EditorCtx[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable] struct {
	Token     *TToken
	TokenRole *TTokenRole
	NodeKind  *TNodeKind

	IsNest           bool
	NestLabel        syntaxa.GrammarLabel
	IsInvalidContext bool
}

/*
EditorOverride allows replacing the default pattern, context, or behavior for a token.

Pattern overrides the lexer rule pattern. MatchContext and Captures override the
produced context. ForeignMachinePayload and DelimitedPayload enable embedded
machines or delimited regions instead of the default transition.
*/
type EditorOverride[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	Pattern          *pattern.RegulaAST[TObservation]
	PatternRegex     *string
	MatchContext     *TContext
	Captures         map[int]TContext
	ForeignPayload   *ForeignMachinePayload[TObservation, TContext]
	DelimitedPayload *DelimitedPayload[TObservation, TContext]
}

/*
EditorIRConfiguration holds callbacks used to build EditorIR from a StateGraph.

tokenFormatter produces deterministic keys for tokens. contextProducer maps
EditorCtx to TContext for each state and transition. overrideProducer may return
an EditorOverride to replace pattern/context or use delimited/foreign payloads.
contextsEqual is used to deduplicate states by context.
*/
type EditorIRConfiguration[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	hasher *hash.XXH3Hasher

	tokenHasher      func(token TToken) uint64
	contextProducer  func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext
	overrideProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) []*EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
	contextsEqual    func(left, right TContext) bool
}

/*
EditorIRConfigurationCreate allocates and returns an EditorIRConfiguration with the given callbacks.

All parameters must be non-nil when the configuration is used to build EditorIR.
*/
func EditorIRConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	hasher *hash.XXH3Hasher,
	tokenHasher func(token TToken) uint64,
	contextProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext,
	overrideProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) []*EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	contextsEqual func(left, right TContext) bool,
) *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		hasher:           hasher,
		tokenHasher:      tokenHasher,
		contextProducer:  contextProducer,
		overrideProducer: overrideProducer,
		contextsEqual:    contextsEqual,
	}
}

/*
EditorState is a single state in the editor IR state machine.

ID and Label identify the state. Context is the semantic context (e.g. highlighting
scope). Transitions are the outgoing edges. HasFallthroughPop and FallthroughPopAmount
support optional-continuation fallthrough. ImmediatePushTarget is set for nest-body
wrapper states so the engine pushes the content state immediately.

HasFallbackInvalid is true when this state has transitions and does not have a
fallthrough-pop (which handles unexpected input by popping to the parent). When set,
FallbackInvalidContext holds the pre-computed context for a synthesised catch-all
transition that should match any input not covered by the explicit transitions and
apply the error scope (e.g. "invalid.illegal.unexpected-token"). Backends must emit
such a catch-all rule when HasFallbackInvalid is true, ensuring no input region is
ever left unscoped (i.e. no "source"-only fallthrough).
*/
type EditorState[TObservation cmp.Ordered, TContext any] struct {
	ID                     string
	Label                  string
	Context                TContext
	Transitions            []EditorTransition[TObservation, TContext]
	HasFallthroughPop      bool
	FallthroughPopAmount   int
	ImmediatePushTarget    *EditorState[TObservation, TContext]
	HasFallbackInvalid     bool
	FallbackInvalidContext TContext
}

/*
StackOperation is the stack operation for an editor transition.

STACK_NONE (Match): consume token, no stack change. STACK_PUSH: push a state.
STACK_POP: pop PopAmount frames. STACK_SET: replace top. STACK_EMBED: enter a
foreign machine (see ForeignMachinePayload).
*/
type StackOperation uint8

const (
	STACK_NONE StackOperation = iota
	STACK_PUSH
	STACK_POP
	STACK_EMBED
	STACK_SET
)

/*
DelimitedPayload describes a delimited region override (open/body/close).

StateLabel names the synthetic state. BodyContext is the context inside the region.
ClosePattern and CloseContext define the closing token and its context.
*/
type DelimitedPayload[TObservation cmp.Ordered, TContext any] struct {
	StateLabel   string
	BodyContext  TContext
	ClosePattern pattern.RegulaAST[TObservation]
	CloseContext TContext
}

/*
ForeignMachinePayload describes an embedded foreign machine (e.g. inline regex).

MachineID identifies the machine. MachineContext is the context for the embedded
state. EscapePattern and EscapeCaptures define how to leave the machine.
*/
type ForeignMachinePayload[TObservation cmp.Ordered, TContext any] struct {
	MachineID      string
	MachineContext TContext
	EscapePattern  pattern.RegulaAST[TObservation]
	EscapeCaptures map[int]TContext
}

/*
EditorTransition is a single outgoing edge from an editor state.

OnPattern is matched against the observation stream; MatchContext is the context
when matched. Targets are the successor states (for Push/Set). Operation and
PopAmount define stack behavior. Captures map capture indices to contexts.
ForeignPayload is set for STACK_EMBED. IsLookahead indicates lookahead-only match.
*/
type EditorTransition[TObservation cmp.Ordered, TContext any] struct {
	OnPattern      pattern.RegulaAST[TObservation]
	RegexPattern   *string
	MatchContext   TContext
	Targets        []*EditorState[TObservation, TContext]
	Captures       map[int]TContext
	Operation      StackOperation
	ForeignPayload *ForeignMachinePayload[TObservation, TContext]
	PopAmount      int
	IsLookahead    bool
}

/*
LanguageMeta holds language identification for the editor IR.

LanguageName and LanguageVersion are set from the grammar package and used by
backends (e.g. Sublime syntax generator) for file headers.
*/
type LanguageMeta struct {
	LanguageName    string
	LanguageVersion string
}

/*
LanguageMachine is the state machine portion of the editor IR.

EditorStates are all states (including synthetic nest-body and delimited states).
RootState is the entry state. AmbientTransitions are transitions from the root
for tokens not in the grammar (e.g. comments).
*/
type LanguageMachine[TObservation cmp.Ordered, TContext any] struct {
	EditorStates       []EditorState[TObservation, TContext]
	RootState          EditorState[TObservation, TContext]
	AmbientTransitions []EditorTransition[TObservation, TContext]
}

/*
EditorIR is the complete editor intermediate representation for syntax highlighting.

It embeds LanguageMeta and LanguageMachine. Built from a GrammarPackage and
EditorIRConfiguration via EditorIRCreate or EditorIRFromStateGraph. Consumed by
backends (e.g. langspec/editor/sublime) to generate syntax definitions.
*/
type EditorIR[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	LanguageMeta
	LanguageMachine[TObservation, TContext]
}

/*
EditorIRCreate builds EditorIR from a grammar package and configuration.

Calls lowering.BuildStateGraph then EditorIRFromStateGraph. Use when you have a
GrammarPackage and lexing ruleset; the state graph is not exposed.

Prerequisites:
- lexingRuleset, grammarPackage, and config must be non-nil.

Edge cases:
- Returns an error if BuildStateGraph or EditorIRFromStateGraph fails.
*/
func EditorIRCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) (*EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], error) {
	sg, err := lowering.BuildStateGraph(grammarPackage, config.tokenHasher, config.hasher)
	if err != nil {
		return nil, err
	}
	return EditorIRFromStateGraph(sg, lexingRuleset, config, grammarPackage)
}
