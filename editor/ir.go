package editor

import (
	"autarch"
	"autarch/pattern"
	"cmp"
	"essence"
	"fmt"
	"foundation/text"
	"lexarch"
	"memarch"
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
	contextsEqual    func(left, right TContext) bool

	editorPDAAllocFn memarch.AllocationFn
}

func EditorIRConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	tokenFormatter func(token TToken) string,
	contextProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext,
	overrideProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) (override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], hasOverride bool),
	contextsEqual func(left, right TContext) bool,
	editorPDAAllocFn memarch.AllocationFn,
) *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		tokenFormatter:   tokenFormatter,
		contextProducer:  contextProducer,
		overrideProducer: overrideProducer,
		contextsEqual:    contextsEqual,
		editorPDAAllocFn: editorPDAAllocFn,
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
	STACK_NONE StackOperation = iota
	STACK_PUSH
	STACK_POP
	STACK_EMBED
	STACK_SET
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
	Targets        []*EditorState[TObservation, TContext]
	Captures       map[int]TContext
	Operation      StackOperation
	ForeignPayload *ForeignMachinePayload[TObservation, TContext]
	PopAmount      int
	IsLookahead    bool
}

type LanguageMeta struct {
	LanguageName    string
	LanguageVersion string
}

type LanguageMachine[TObservation cmp.Ordered, TContext any] struct {
	EditorStates       []EditorState[TObservation, TContext]
	RootState          EditorState[TObservation, TContext]
	AmbientTransitions []EditorTransition[TObservation, TContext]
}

type EditorIR[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	LanguageMeta
	LanguageMachine[TObservation, TContext]
}

// ------------------------------------------------------------- Generator

type editorIRGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	lexingRuleset  *lexarch.LexingRuleset[TObservation, TToken, TTokenRole]
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	config         *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
	sanitizer      *text.Sanitizer

	tokenToRule    map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]
	compiledStates map[autarch.StackSymbolID]*EditorState[TObservation, TContext]

	delimitedStates map[string]*EditorState[TObservation, TContext]
}

func EditorIRCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	pdaConfig syntaxa.NPDAConfig,
) (*EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], error) {

	generator := &editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		lexingRuleset:   lexingRuleset,
		grammarPackage:  grammarPackage,
		config:          config,
		sanitizer:       text.NewIdentifierSanitizer(),
		tokenToRule:     make(map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]),
		compiledStates:  make(map[autarch.StackSymbolID]*EditorState[TObservation, TContext]),
		delimitedStates: make(map[string]*EditorState[TObservation, TContext]),
	}

	generator.buildTokenMap()

	engine, err := syntaxa.CompileEngine[TObservation, TToken, TTokenRole, TNodeKind, TLexerState, TContext](
		grammarPackage, config.editorPDAAllocFn, pdaConfig,
	)
	if err != nil {
		return nil, err
	}

	if !engine.IsDPDA {
		autarch.NPDADestroy(engine.NPDA)
		panic(fmt.Sprintf(
			"EditorIR generator currently requires a strict LL(1) DPDA.\nResolve grammar ambiguities:\n%v",
			engine.DPDAError,
		))
	}

	return &EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		LanguageMeta: LanguageMeta{
			LanguageName:    grammarPackage.Name,
			LanguageVersion: grammarPackage.Version,
		},
		LanguageMachine: LanguageMachine[TObservation, TContext]{
			EditorStates:       finalStates,
			RootState:          *rootState,
			AmbientTransitions: ambientTransitions,
		},
	}, nil
}

// ------------------------------------------------------------- Private Helpers

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTokenMap() {
	for _, rule := range e.lexingRuleset.GetRules() {
		e.tokenToRule[rule.Token] = rule
	}
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) resolveLabel(
	stackTop autarch.StackSymbolID,
	engine *syntaxa.PDAEngine[TToken, TContext],
) string {
	debugName, exists := engine.DebugMap[stackTop]
	if !exists {
		return e.generateID()
	}

	cleanName := e.sanitizer.Sanitize(debugName)

	if nodeKey, hasKey := engine.StackToNode[stackTop]; hasKey {
		cleanPath := e.sanitizer.Sanitize(string(nodeKey))
		return fmt.Sprintf("%s__%s", cleanName, cleanPath)
	}

	if cleanName != "" {
		return cleanName
	}
	return e.generateID()
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) generateID() string {
	id, _ := essence.UUIDv7GenerateMonotonic()
	return id.String()
}
