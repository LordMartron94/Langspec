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

type EditorIRConfiguration[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	tokenFormatter  func(token TToken) string
	contextProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext
}

func EditorIRConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	tokenFormatter func(token TToken) string,
	contextProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext,
) *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		tokenFormatter:  tokenFormatter,
		contextProducer: contextProducer,
	}
}

type EditorState[TObservation cmp.Ordered, TContext any] struct {
	ID          string
	Label       string
	Context     TContext // The scope applied to the environment as a whole (if any)
	Transitions []EditorTransition[TObservation, TContext]
}

type StackOperation uint8

const (
	STACK_NONE StackOperation = iota // Token matched, no state change
	STACK_PUSH                       // Token matched, enter new Target state
	STACK_POP                        // Token matched, exit current state
)

type EditorTransition[TObservation cmp.Ordered, TContext any] struct {
	OnPattern    pattern.RegulaAST[TObservation]
	MatchContext TContext
	Target       *EditorState[TObservation, TContext]
	Operation    StackOperation
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

	rootState := generator.buildRootState()

	return &EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		LanguageMeta: LanguageMeta{
			LanguageName:    grammarPackage.Name,
			LanguageVersion: grammarPackage.Version,
		},
		LanguageMachine: LanguageMachine[TObservation, TContext]{
			EditorStates: []EditorState[TObservation, TContext]{rootState},
			RootState:    rootState,
		},
	}
}

// ------------------------------------------------------------- Private Helpers

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildRootState() EditorState[TObservation, TContext] {
	sortedRules := e.getSortedLexerRules()
	transitions := e.buildTransitions(sortedRules)

	return EditorState[TObservation, TContext]{
		ID:          e.generateID(),
		Label:       ROOT_LABEL,
		Context:     *new(TContext),
		Transitions: transitions,
	}
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getSortedLexerRules() []lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole] {
	rules := e.lexingRuleset.GetRules()

	return extensions.SortedCopyShallow(rules, func(a, b lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]) int {
		return cmp.Compare(b.Priority, a.Priority)
	})
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTransitions(
	rules []lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
) []EditorTransition[TObservation, TContext] {

	transitions := make([]EditorTransition[TObservation, TContext], len(rules))

	for i, rule := range rules {
		transitions[i] = e.buildSingleTransition(rule)
	}

	return transitions
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildSingleTransition(
	rule lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
) EditorTransition[TObservation, TContext] {

	ctx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		Token:     &rule.Token,
		TokenRole: &rule.Role,
	}

	return EditorTransition[TObservation, TContext]{
		OnPattern:    rule.Pattern,
		MatchContext: e.config.contextProducer(ctx),
		Target:       nil,
		Operation:    STACK_NONE,
	}
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) generateID() string {
	id, _ := essence.UUIDv7GenerateMonotonic()
	return id.String()
}
