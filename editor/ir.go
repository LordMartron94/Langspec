package editor

import (
	"autarch/pattern"
	"cmp"
	"essence"
	"foundation/extensions"
	"foundation/text"
	"lexarch"
	"syntaxa"
)

const ROOT_LABEL = "root"

// ------------------------------------------------------------- TYPES

type EditorCtx[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable] struct {
	Token     *TToken
	TokenRole *TTokenRole
}

type EditorOverride[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	Pattern          *pattern.RegulaAST[TObservation]
	MatchContext     *TContext
	Captures         map[int]TContext
	ForeignPayload   *ForeignMachinePayload[TObservation, TContext]
	DelimitedPayload *DelimitedPayload[TObservation, TContext]
}

type EditorIRConfiguration[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	tokenFormatter   func(token TToken) string
	contextProducer  func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext
	overrideProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) (override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], hasOverride bool)
}

func EditorIRConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	tokenFormatter func(token TToken) string,
	contextProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext,
	overrideProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) (override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], hasOverride bool),
) *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		tokenFormatter:   tokenFormatter,
		contextProducer:  contextProducer,
		overrideProducer: overrideProducer,
	}
}

type EditorState[TObservation cmp.Ordered, TContext any] struct {
	ID          string
	Label       string
	Context     TContext
	Transitions []EditorTransition[TObservation, TContext]
}

type StackOperation uint8

const (
	STACK_NONE StackOperation = iota // Token matched, no state change
	STACK_PUSH                       // Token matched, enter new Target state
	STACK_POP                        // Token matched, exit current state
	STACK_EMBED
)

type DelimitedPayload[TObservation cmp.Ordered, TContext any] struct {
	StateLabel   string
	BodyContext  TContext
	ClosePattern pattern.RegulaAST[TObservation]
	CloseContext TContext
}

type ForeignMachinePayload[TObservation cmp.Ordered, TContext any] struct {
	MachineID      string
	MachineContext TContext
	EscapePattern  pattern.RegulaAST[TObservation]
	EscapeCaptures map[int]TContext
}

type EditorTransition[TObservation cmp.Ordered, TContext any] struct {
	OnPattern      pattern.RegulaAST[TObservation]
	MatchContext   TContext
	Target         *EditorState[TObservation, TContext]
	Captures       map[int]TContext
	Operation      StackOperation
	ForeignPayload *ForeignMachinePayload[TObservation, TContext]
}

type LanguageMeta struct {
	LanguageName    string
	LanguageVersion string
}

type LanguageMachine[TObservation cmp.Ordered, TContext any] struct {
	EditorStates []EditorState[TObservation, TContext]
	RootState    EditorState[TObservation, TContext]
}

type EditorIR[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	LanguageMeta
	LanguageMachine[TObservation, TContext]
}

// ------------------------------------------------------------- Generator

type editorIRGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	lexingRuleset  *lexarch.LexingRuleset[TObservation, TToken, TTokenRole]
	grammarPackage syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	config         *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
	sanitizer      *text.Sanitizer
}

func EditorIRCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) *EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {

	generator := &editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		lexingRuleset:  lexingRuleset,
		grammarPackage: grammarPackage,
		config:         config,
		sanitizer:      text.NewIdentifierSanitizer(),
	}

	states, rootState := generator.buildRootState()

	editorStates := []EditorState[TObservation, TContext]{rootState}
	editorStates = append(editorStates, states...)

	return &EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		LanguageMeta: LanguageMeta{
			LanguageName:    grammarPackage.Name,
			LanguageVersion: grammarPackage.Version,
		},
		LanguageMachine: LanguageMachine[TObservation, TContext]{
			EditorStates: editorStates,
			RootState:    rootState,
		},
	}
}

// ------------------------------------------------------------- Private Helpers

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildRootState() ([]EditorState[TObservation, TContext], EditorState[TObservation, TContext]) {
	sortedRules := e.getSortedLexerRules()
	transitions, extraStates := e.buildTransitions(sortedRules)

	rootState := EditorState[TObservation, TContext]{
		ID:          e.generateID(),
		Label:       ROOT_LABEL,
		Context:     *new(TContext),
		Transitions: transitions,
	}

	return extraStates, rootState
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getSortedLexerRules() []lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole] {
	rules := e.lexingRuleset.GetRules()

	return extensions.SortedCopyShallow(rules, func(a, b lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]) int {
		return cmp.Compare(b.Priority, a.Priority)
	})
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTransitions(
	rules []lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
) ([]EditorTransition[TObservation, TContext], []EditorState[TObservation, TContext]) {

	var transitions []EditorTransition[TObservation, TContext]
	var extraStates []EditorState[TObservation, TContext]

	for _, rule := range rules {
		t, targetState := e.buildSingleTransition(rule)
		transitions = append(transitions, t)
		if targetState != nil {
			extraStates = append(extraStates, *targetState)
		}
	}

	return transitions, extraStates
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildSingleTransition(
	rule lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
) (EditorTransition[TObservation, TContext], *EditorState[TObservation, TContext]) {

	ctx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		Token:     &rule.Token,
		TokenRole: &rule.Role,
	}

	pattern := rule.Pattern
	matchCtx := e.config.contextProducer(ctx)

	t := EditorTransition[TObservation, TContext]{
		Operation: STACK_NONE,
	}
	var spawnedState *EditorState[TObservation, TContext]

	if override, hasOverride := e.config.overrideProducer(ctx); hasOverride {
		pattern = e.applyPatternOverride(pattern, override)
		matchCtx = e.applyContextOverride(matchCtx, override)
		t.Captures = override.Captures

		if override.ForeignPayload != nil {
			t.ForeignPayload = override.ForeignPayload
			t.Operation = STACK_EMBED
		} else if override.DelimitedPayload != nil {
			spawnedState = e.constructDelimitedTargetState(override.DelimitedPayload)
			t.Target = spawnedState
			t.Operation = STACK_PUSH
		}
	}

	t.OnPattern = pattern
	t.MatchContext = matchCtx

	return t, spawnedState
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) constructDelimitedTargetState(
	payload *DelimitedPayload[TObservation, TContext],
) *EditorState[TObservation, TContext] {

	return &EditorState[TObservation, TContext]{
		ID:      e.generateID(),
		Label:   payload.StateLabel,
		Context: payload.BodyContext,
		Transitions: []EditorTransition[TObservation, TContext]{
			{
				OnPattern:    payload.ClosePattern,
				MatchContext: payload.CloseContext,
				Target:       nil,
				Operation:    STACK_POP,
			},
		},
	}
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) applyPatternOverride(
	base pattern.RegulaAST[TObservation],
	override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) pattern.RegulaAST[TObservation] {
	if override.Pattern != nil {
		return *override.Pattern
	}
	return base
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) applyContextOverride(
	base TContext,
	override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) TContext {
	if override.MatchContext != nil {
		return *override.MatchContext
	}
	return base
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) generateID() string {
	id, _ := essence.UUIDv7GenerateMonotonic()
	return id.String()
}
