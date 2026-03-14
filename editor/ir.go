package editor

import (
	"autarch/pattern"
	"cmp"
	"fmt"
	"foundation/text"
	"lexarch"
	"sort"
	"strings"
	"syntaxa"
)

const ROOT_LABEL = "root"

type EditorCtx[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable] struct {
	Token     *TToken
	TokenRole *TTokenRole
	NodeKind  *TNodeKind

	IsNest    bool
	NestLabel syntaxa.GrammarLabel
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
}

func EditorIRConfigurationCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	tokenFormatter func(token TToken) string,
	contextProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) TContext,
	overrideProducer func(editorCtx *EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) (override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], hasOverride bool),
	contextsEqual func(left, right TContext) bool,
) *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		tokenFormatter:   tokenFormatter,
		contextProducer:  contextProducer,
		overrideProducer: overrideProducer,
		contextsEqual:    contextsEqual,
	}
}

type EditorState[TObservation cmp.Ordered, TContext any] struct {
	ID                   string
	Label                string
	Context              TContext
	Transitions          []EditorTransition[TObservation, TContext]
	HasFallthroughPop    bool
	FallthroughPopAmount int
	ImmediatePushTarget  *EditorState[TObservation, TContext]
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

type lsStackEntry[TToken, TNodeKind comparable] struct {
	isRepetition bool
	label        syntaxa.GrammarLabel
	repeatNode   *syntaxa.Grammar[TToken, TNodeKind]
	remaining    []*syntaxa.Grammar[TToken, TNodeKind]
}

type lsTerminal[TToken, TNodeKind comparable] struct {
	token     TToken
	remaining []*syntaxa.Grammar[TToken, TNodeKind]
	stack     []lsStackEntry[TToken, TNodeKind]
	nestNode  *syntaxa.Grammar[TToken, TNodeKind]
	nodeKind  *TNodeKind
	popOffset int
}

func (t *lsTerminal[TToken, TNodeKind]) getLastRemaining() *[]*syntaxa.Grammar[TToken, TNodeKind] {
	if len(t.stack) == 0 {
		return &t.remaining
	}
	return &t.stack[len(t.stack)-1].remaining
}

type lsVisiting map[syntaxa.GrammarLabel]bool

func lsIsSelfRecursive[TToken, TNodeKind comparable](
	label syntaxa.GrammarLabel,
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
) bool {
	if label == "" {
		return false
	}
	root := rules[label]
	if root == nil {
		return false
	}
	return lsNodeContainsRef(root, label)
}

func lsNodeContainsRef[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
	target syntaxa.GrammarLabel,
) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case syntaxa.GReference:
		return node.ReferenceTarget == target
	case syntaxa.GConcat, syntaxa.GChoice:
		for _, child := range node.Children {
			if lsNodeContainsRef(child, target) {
				return true
			}
		}
	case syntaxa.GRepeat, syntaxa.GOptional:
		if len(node.Children) > 0 {
			return lsNodeContainsRef(node.Children[0], target)
		}
	}
	return false
}

func lsLookahead[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) ([]lsTerminal[TToken, TNodeKind], bool) {
	if node == nil {
		return nil, true
	}

	switch node.Kind {
	case syntaxa.GToken:
		return []lsTerminal[TToken, TNodeKind]{{token: node.Token, nodeKind: node.OutputNodeKind}}, false
	case syntaxa.GEpsilon:
		return nil, true
	case syntaxa.GConcat:
		return lsLookaheadConcat(node.Children, rules, visiting)
	case syntaxa.GChoice:
		return lsLookaheadChoice(node, rules, visiting)
	case syntaxa.GRepeat, syntaxa.GOptional:
		return lsLookaheadRepetition(node, rules, visiting)
	case syntaxa.GReference:
		return lsLookaheadReference(node, rules, visiting)
	case syntaxa.GNest:
		return lsLookaheadNest(node)
	}

	return nil, false
}

func lsLookaheadChoice[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) ([]lsTerminal[TToken, TNodeKind], bool) {
	var all []lsTerminal[TToken, TNodeKind]
	anyNull := false
	for _, child := range node.Children {
		ts, n := lsLookahead(child, rules, visiting)
		all = append(all, ts...)
		anyNull = anyNull || n
	}
	return all, anyNull
}

func lsLookaheadRepetition[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) ([]lsTerminal[TToken, TNodeKind], bool) {
	child := node.Children[0]
	ts, _ := lsLookahead(child, rules, visiting)
	for i := range ts {
		ts[i].stack = append(ts[i].stack, lsStackEntry[TToken, TNodeKind]{
			isRepetition: true,
			repeatNode:   node,
			label:        node.GrammarLabel,
		})
	}
	return ts, node.Min == 0 || node.Kind == syntaxa.GOptional
}

func lsLookaheadReference[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) ([]lsTerminal[TToken, TNodeKind], bool) {
	target := node.ResolvedReference
	if target == nil {
		target = rules[node.ReferenceTarget]
	}
	if target == nil || visiting[node.ReferenceTarget] {
		return nil, false
	}
	visiting[node.ReferenceTarget] = true
	ts, n := lsLookahead(target, rules, visiting)
	delete(visiting, node.ReferenceTarget)
	for i := range ts {
		ts[i].stack = append(ts[i].stack, lsStackEntry[TToken, TNodeKind]{
			isRepetition: false,
			label:        node.ReferenceTarget,
		})
	}
	return ts, n
}

func lsLookaheadNest[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
) ([]lsTerminal[TToken, TNodeKind], bool) {
	t := lsTerminal[TToken, TNodeKind]{
		token:    *node.OpenToken,
		nestNode: node,
	}
	return []lsTerminal[TToken, TNodeKind]{t}, false
}

func lsLookaheadConcat[TToken, TNodeKind comparable](
	nodes []*syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) ([]lsTerminal[TToken, TNodeKind], bool) {
	nullable := true
	var terms []lsTerminal[TToken, TNodeKind]

	for i, node := range nodes {
		ts, n := lsLookahead(node, rules, visiting)
		suffix := nodes[i+1:]

		for j := range ts {
			last := ts[j].getLastRemaining()
			extended := make([]*syntaxa.Grammar[TToken, TNodeKind], len(*last)+len(suffix))
			copy(extended, *last)
			copy(extended[len(*last):], suffix)
			*last = extended
		}

		terms = append(terms, ts...)

		if !n {
			nullable = false
			break
		}
	}
	return terms, nullable
}

func lsAdvanceTerminal[TToken, TNodeKind comparable](
	term lsTerminal[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
) []lsTerminal[TToken, TNodeKind] {
	levels := len(term.stack) + 1
	var result []lsTerminal[TToken, TNodeKind]

	for i := 0; i < levels; i++ {
		remaining, isRep, repNode := extractFrameDetails(term, i)

		if len(remaining) == 0 && !isRep {
			continue
		}

		la, nullable := computeLookaheadForFrame(remaining, isRep, repNode, rules)
		result = appendLookaheadWithStack(la, term.stack[i:], result)

		if !nullable {
			break
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func extractFrameDetails[TToken, TNodeKind comparable](
	term lsTerminal[TToken, TNodeKind],
	level int,
) ([]*syntaxa.Grammar[TToken, TNodeKind], bool, *syntaxa.Grammar[TToken, TNodeKind]) {
	if level == 0 {
		return term.remaining, false, nil
	}
	entry := term.stack[level-1]
	return entry.remaining, entry.isRepetition, entry.repeatNode
}

func computeLookaheadForFrame[TToken, TNodeKind comparable](
	remaining []*syntaxa.Grammar[TToken, TNodeKind],
	isRep bool,
	repNode *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
) ([]lsTerminal[TToken, TNodeKind], bool) {
	visiting := make(lsVisiting)
	if isRep {
		loopNodes := make([]*syntaxa.Grammar[TToken, TNodeKind], 0, 1+len(remaining))
		loopNodes = append(loopNodes, repNode)
		loopNodes = append(loopNodes, remaining...)
		return lsLookaheadConcat(loopNodes, rules, visiting)
	}
	return lsLookaheadConcat(remaining, rules, visiting)
}

func appendLookaheadWithStack[TToken, TNodeKind comparable](
	la []lsTerminal[TToken, TNodeKind],
	outerStack []lsStackEntry[TToken, TNodeKind],
	result []lsTerminal[TToken, TNodeKind],
) []lsTerminal[TToken, TNodeKind] {
	for j := range la {
		newStack := make([]lsStackEntry[TToken, TNodeKind], len(la[j].stack)+len(outerStack))
		copy(newStack, la[j].stack)
		copy(newStack[len(la[j].stack):], outerStack)
		la[j].stack = newStack
	}
	return append(result, la...)
}

func lsLookaheadKey[TToken, TNodeKind comparable](
	terms []lsTerminal[TToken, TNodeKind],
	tokenFmt func(TToken) string,
) string {
	keys := make([]string, len(terms))
	for i, t := range terms {
		keys[i] = lsTerminalKey(t, tokenFmt)
	}
	sort.Strings(keys)
	return strings.Join(keys, "|")
}

func lsTerminalKey[TToken, TNodeKind comparable](
	t lsTerminal[TToken, TNodeKind],
	tokenFmt func(TToken) string,
) string {
	var sb strings.Builder
	sb.WriteString(tokenFmt(t.token))
	if t.nestNode != nil {
		sb.WriteString(":NEST(")
		sb.WriteString(string(t.nestNode.GrammarLabel))
		sb.WriteString(")")
	}
	sb.WriteString(":(")
	sb.WriteString(lsGrammarNodesKey(t.remaining, tokenFmt))
	sb.WriteString(")")

	seenLabels := make(map[syntaxa.GrammarLabel]bool)
	seenReps := make(map[*syntaxa.Grammar[TToken, TNodeKind]]bool)

	for _, e := range t.stack {
		if e.isRepetition {
			if seenReps[e.repeatNode] {
				break
			}
			seenReps[e.repeatNode] = true

			sb.WriteString(":R[")
			sb.WriteString(lsGrammarNodesKey(e.remaining, tokenFmt))
			sb.WriteString("]")
		} else {
			if e.label != "" && seenLabels[e.label] {
				break
			}
			if e.label != "" {
				seenLabels[e.label] = true
			}

			sb.WriteString(":V(")
			sb.WriteString(string(e.label))
			sb.WriteString(",")
			sb.WriteString(lsGrammarNodesKey(e.remaining, tokenFmt))
			sb.WriteString(")")
		}
	}
	if t.popOffset > 0 {
		fmt.Fprintf(&sb, ":PO%d", t.popOffset)
	}
	return sb.String()
}

func lsGrammarNodesKey[TToken, TNodeKind comparable](
	nodes []*syntaxa.Grammar[TToken, TNodeKind],
	tokenFmt func(TToken) string,
) string {
	parts := make([]string, len(nodes))
	for i, n := range nodes {
		parts[i] = lsGrammarNodeKey(n, tokenFmt)
	}
	return strings.Join(parts, ",")
}

func lsGrammarNodeKey[TToken, TNodeKind comparable](
	n *syntaxa.Grammar[TToken, TNodeKind],
	tokenFmt func(TToken) string,
) string {
	if n == nil {
		return "nil"
	}
	if n.NodePath != nil {
		return string(*n.NodePath)
	}
	if n.Kind == syntaxa.GToken {
		return "ST:" + tokenFmt(n.Token)
	}
	return string(n.GrammarLabel)
}

func lsOwningRule[TToken, TNodeKind comparable](term lsTerminal[TToken, TNodeKind]) syntaxa.GrammarLabel {
	for i := 0; i < len(term.stack); i++ {
		if !term.stack[i].isRepetition && term.stack[i].label != "" {
			return term.stack[i].label
		}
	}
	for i := 0; i < len(term.stack); i++ {
		if term.stack[i].isRepetition && term.stack[i].label != "" {
			return term.stack[i].label
		}
	}
	return "anon"
}

type editorIRGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	lexingRuleset   *lexarch.LexingRuleset[TObservation, TToken, TTokenRole]
	grammarPackage  *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	config          *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
	sanitizer       *text.Sanitizer
	tokenToRule     map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]
	delimitedStates map[string]*EditorState[TObservation, TContext]
}

type lsBuildState[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	gen          *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
	rules        map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind]
	contextByKey map[string]*EditorState[TObservation, TContext]
	states       []*EditorState[TObservation, TContext]
	counters     map[string]int
	queue        []lsPendingCtx[TObservation, TToken, TNodeKind, TContext]
}

type lsPendingCtx[TObservation cmp.Ordered, TToken, TNodeKind comparable, TContext any] struct {
	state     *EditorState[TObservation, TContext]
	ctxKey    string
	terminals []lsTerminal[TToken, TNodeKind]
	nameHint  syntaxa.GrammarLabel
}

func EditorIRCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) (*EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], error) {

	gen := buildGenerator(lexingRuleset, grammarPackage, config)
	bs := buildState(gen, grammarPackage)

	if err := initializeBuildQueue(bs, grammarPackage); err != nil {
		return nil, err
	}

	processQueue(bs)
	ambientTransitions := lsBuildAmbientTransitions(gen, grammarPackage.Grammars, bs)
	allStates, rootOut := collectStates(bs, gen)

	return &EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		LanguageMeta: LanguageMeta{
			LanguageName:    grammarPackage.Name,
			LanguageVersion: grammarPackage.Version,
		},
		LanguageMachine: LanguageMachine[TObservation, TContext]{
			EditorStates:       allStates,
			RootState:          rootOut,
			AmbientTransitions: ambientTransitions,
		},
	}, nil
}

func buildGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	gen := &editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		lexingRuleset:   lexingRuleset,
		grammarPackage:  grammarPackage,
		config:          config,
		sanitizer:       text.NewIdentifierSanitizer(),
		tokenToRule:     make(map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]),
		delimitedStates: make(map[string]*EditorState[TObservation, TContext]),
	}
	gen.buildTokenMap()
	return gen
}

func buildState[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	gen *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
) *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	return &lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		gen:          gen,
		rules:        grammarPackage.Grammars,
		contextByKey: make(map[string]*EditorState[TObservation, TContext]),
		counters:     make(map[string]int),
	}
}

func initializeBuildQueue[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
) error {
	entryLabel := grammarPackage.EntryRule
	entryRule := grammarPackage.Grammars[entryLabel]
	if entryRule == nil {
		return fmt.Errorf("EditorIRCreate: entry rule %q not found", entryLabel)
	}

	entryTerminals, _ := lsLookahead[TToken, TNodeKind](entryRule, bs.rules, make(lsVisiting))
	rootState := &EditorState[TObservation, TContext]{ID: ROOT_LABEL, Label: ROOT_LABEL}
	rootKey := lsLookaheadKey(entryTerminals, bs.gen.config.tokenFormatter)

	bs.contextByKey[rootKey] = rootState
	bs.states = append(bs.states, rootState)
	bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
		state:     rootState,
		ctxKey:    rootKey,
		terminals: entryTerminals,
	})
	return nil
}

func processQueue[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) {
	for len(bs.queue) > 0 {
		pending := bs.queue[0]
		bs.queue = bs.queue[1:]
		bs.processContext(pending)
	}
}

func collectStates[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	gen *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) ([]EditorState[TObservation, TContext], EditorState[TObservation, TContext]) {
	delimitedStateKeys := make(map[string]bool, len(gen.delimitedStates))
	for label := range gen.delimitedStates {
		delimitedStateKeys[label] = true
	}

	allStates := make([]EditorState[TObservation, TContext], 0, len(bs.states)+len(gen.delimitedStates))
	var rootOut EditorState[TObservation, TContext]

	for _, s := range bs.states {
		allStates = append(allStates, *s)
		if s.Label == ROOT_LABEL {
			rootOut = *s
		}
		delete(delimitedStateKeys, s.Label)
	}

	for label := range delimitedStateKeys {
		if s, ok := gen.delimitedStates[label]; ok {
			allStates = append(allStates, *s)
		}
	}
	return allStates, rootOut
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) processContext(
	pending lsPendingCtx[TObservation, TToken, TNodeKind, TContext],
) {
	sort.SliceStable(pending.terminals, func(i, j int) bool {
		return bs.gen.tokenPriority(pending.terminals[i].token) >
			bs.gen.tokenPriority(pending.terminals[j].token)
	})

	for _, term := range pending.terminals {
		tr := bs.buildTransition(term, pending.nameHint)
		if tr != nil {
			pending.state.Transitions = append(pending.state.Transitions, *tr)
		}
	}
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getOrCreateContext(
	terminals []lsTerminal[TToken, TNodeKind],
	ownerLabel syntaxa.GrammarLabel,
	nameHint syntaxa.GrammarLabel,
) *EditorState[TObservation, TContext] {
	key := lsLookaheadKey(terminals, bs.gen.config.tokenFormatter)
	if s, ok := bs.contextByKey[key]; ok {
		return s
	}

	effectiveLabel := ownerLabel
	if (effectiveLabel == "anon" || effectiveLabel == "") && nameHint != "" {
		effectiveLabel = nameHint
	}
	base := bs.gen.sanitizer.Sanitize(string(effectiveLabel))
	if base == "" {
		base = "ctx"
	}
	n := bs.counters[base]
	bs.counters[base] = n + 1
	name := fmt.Sprintf("%s__%d", base, n)

	s := &EditorState[TObservation, TContext]{ID: name, Label: name}

	if lsAllOptionalTerminals(terminals) {
		s.HasFallthroughPop = true
		if len(terminals) > 0 && terminals[0].popOffset > 0 {
			s.FallthroughPopAmount = 1 + terminals[0].popOffset
		}
	}

	bs.contextByKey[key] = s
	bs.states = append(bs.states, s)
	bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
		state:     s,
		ctxKey:    key,
		terminals: terminals,
		nameHint:  effectiveLabel,
	})
	return s
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTransition(
	term lsTerminal[TToken, TNodeKind],
	nameHint syntaxa.GrammarLabel,
) *EditorTransition[TObservation, TContext] {
	cfg := bs.gen.config
	tokenRule, hasTokenRule := bs.gen.tokenToRule[term.token]

	edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &term.token, NodeKind: term.nodeKind}
	override, hasOverride := cfg.overrideProducer(edCtx)
	matchCtx := cfg.contextProducer(edCtx)

	pat, hasPattern := resolvePattern(override, hasOverride, tokenRule, hasTokenRule)
	if !hasPattern {
		return nil
	}

	if hasOverride && override.MatchContext != nil {
		matchCtx = *override.MatchContext
	}

	captures := resolveCaptures(override, hasOverride)

	if hasOverride && override.ForeignPayload != nil {
		return buildForeignTransition(pat, matchCtx, captures, override.ForeignPayload)
	}

	if hasOverride && override.DelimitedPayload != nil {
		return bs.buildDelimitedTransition(pat, matchCtx, captures, override.DelimitedPayload)
	}

	isZeroOrMore, _ := lsOuterFrameKind(term)

	if term.nestNode != nil {
		return bs.buildNestTransition(term, pat, matchCtx, captures, isZeroOrMore)
	}

	return bs.buildStandardTransition(term, pat, matchCtx, captures, nameHint, isZeroOrMore)
}

func resolvePattern[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	hasOverride bool,
	tokenRule lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
	hasTokenRule bool,
) (pattern.RegulaAST[TObservation], bool) {
	if hasOverride && override.Pattern != nil {
		return *override.Pattern, true
	}
	if hasTokenRule {
		return tokenRule.Pattern, true
	}
	var empty pattern.RegulaAST[TObservation]
	return empty, false
}

func resolveCaptures[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	override *EditorOverride[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	hasOverride bool,
) map[int]TContext {
	if hasOverride {
		return override.Captures
	}
	return nil
}

func buildForeignTransition[TObservation cmp.Ordered, TContext any](
	pat pattern.RegulaAST[TObservation],
	matchCtx TContext,
	captures map[int]TContext,
	payload *ForeignMachinePayload[TObservation, TContext],
) *EditorTransition[TObservation, TContext] {
	return &EditorTransition[TObservation, TContext]{
		OnPattern:      pat,
		MatchContext:   matchCtx,
		Captures:       captures,
		Operation:      STACK_EMBED,
		ForeignPayload: payload,
	}
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildDelimitedTransition(
	pat pattern.RegulaAST[TObservation],
	matchCtx TContext,
	captures map[int]TContext,
	payload *DelimitedPayload[TObservation, TContext],
) *EditorTransition[TObservation, TContext] {
	bodyState := bs.gen.getOrCreateDelimitedState(payload)
	return &EditorTransition[TObservation, TContext]{
		OnPattern:    pat,
		MatchContext: matchCtx,
		Captures:     captures,
		Operation:    STACK_PUSH,
		Targets:      []*EditorState[TObservation, TContext]{bodyState},
	}
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildNestTransition(
	term lsTerminal[TToken, TNodeKind],
	pat pattern.RegulaAST[TObservation],
	matchCtx TContext,
	captures map[int]TContext,
	isZeroOrMore bool,
) *EditorTransition[TObservation, TContext] {
	bodyState := bs.getOrCreateNestBodyContext(term.nestNode)
	ownerLabel2 := lsOwningRule(term)
	nestHint := syntaxa.GrammarLabel(string(term.nestNode.GrammarLabel))
	noNest := lsTerminal[TToken, TNodeKind]{token: term.token, remaining: term.remaining, stack: term.stack}
	advTerminals := lsAdvanceTerminal(noNest, bs.rules)

	nestOp := STACK_SET
	if isZeroOrMore {
		nestOp = STACK_PUSH
	}

	if nestOp == STACK_SET {
		lsPropagatePopOffset(advTerminals, term.popOffset)
	}

	if advTerminals == nil {
		return &EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    nestOp,
			Targets:      []*EditorState[TObservation, TContext]{bodyState},
		}
	}

	afterNest := bs.getOrCreateContext(advTerminals, ownerLabel2, nestHint)
	afterNest.HasFallthroughPop = true
	if nestOp == STACK_SET && term.popOffset > 0 && afterNest.FallthroughPopAmount == 0 {
		afterNest.FallthroughPopAmount = 1 + term.popOffset
	}

	return &EditorTransition[TObservation, TContext]{
		OnPattern:    pat,
		MatchContext: matchCtx,
		Captures:     captures,
		Operation:    nestOp,
		Targets:      []*EditorState[TObservation, TContext]{afterNest, bodyState},
	}
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildStandardTransition(
	term lsTerminal[TToken, TNodeKind],
	pat pattern.RegulaAST[TObservation],
	matchCtx TContext,
	captures map[int]TContext,
	nameHint syntaxa.GrammarLabel,
	isZeroOrMore bool,
) *EditorTransition[TObservation, TContext] {
	ownerLabel := lsOwningRule(term)
	var advTerminals []lsTerminal[TToken, TNodeKind]

	if isZeroOrMore {
		stripped := lsTerminal[TToken, TNodeKind]{
			token:     term.token,
			remaining: term.remaining,
			stack:     term.stack[:len(term.stack)-1],
		}
		advTerminals = lsAdvanceTerminal(stripped, bs.rules)
	} else {
		advTerminals = lsAdvanceTerminal(term, bs.rules)
		lsPropagatePopOffset(advTerminals, term.popOffset)
	}

	if isZeroOrMore {
		if advTerminals == nil {
			return &EditorTransition[TObservation, TContext]{
				OnPattern:    pat,
				MatchContext: matchCtx,
				Captures:     captures,
				Operation:    STACK_NONE,
			}
		}
		next := bs.getOrCreateContext(advTerminals, ownerLabel, nameHint)
		return &EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    STACK_PUSH,
			Targets:      []*EditorState[TObservation, TContext]{next},
		}
	}

	if advTerminals == nil {
		return &EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    STACK_POP,
			PopAmount:    1 + term.popOffset,
		}
	}

	next := bs.getOrCreateContext(advTerminals, ownerLabel, nameHint)
	return &EditorTransition[TObservation, TContext]{
		OnPattern:    pat,
		MatchContext: matchCtx,
		Captures:     captures,
		Operation:    STACK_SET,
		Targets:      []*EditorState[TObservation, TContext]{next},
	}
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getOrCreateNestBodyContext(
	nestNode *syntaxa.Grammar[TToken, TNodeKind],
) *EditorState[TObservation, TContext] {
	nestKey := "NEST_BODY:" + string(nestNode.GrammarLabel)
	if s, ok := bs.contextByKey[nestKey]; ok {
		return s
	}

	s := bs.initNestState(nestNode.GrammarLabel, nestKey)
	bodyTerminals := bs.computeNestBodyTerminals(nestNode)
	nestCtx := bs.produceNestContext(nestNode)

	var zero TContext
	if !bs.gen.config.contextsEqual(nestCtx, zero) {
		return bs.setupWrapperContext(s, nestCtx, bodyTerminals, nestNode)
	}

	s.Context = nestCtx
	if len(bodyTerminals) > 0 {
		bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
			state:     s,
			ctxKey:    nestKey,
			terminals: bodyTerminals,
			nameHint:  nestNode.GrammarLabel,
		})
	}

	return s
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) initNestState(
	label syntaxa.GrammarLabel,
	nestKey string,
) *EditorState[TObservation, TContext] {
	name := bs.gen.sanitizer.Sanitize(string(label))
	if name == "" {
		name = "nest_body"
	}

	s := &EditorState[TObservation, TContext]{ID: name, Label: name}
	bs.contextByKey[nestKey] = s
	bs.states = append(bs.states, s)

	return s
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) computeNestBodyTerminals(
	nestNode *syntaxa.Grammar[TToken, TNodeKind],
) []lsTerminal[TToken, TNodeKind] {
	closeTokenNode := &syntaxa.Grammar[TToken, TNodeKind]{Kind: syntaxa.GToken, Token: *nestNode.CloseToken}
	bodyAndClose := []*syntaxa.Grammar[TToken, TNodeKind]{nestNode.Children[0], closeTokenNode}
	terminals, _ := lsLookaheadConcat[TToken, TNodeKind](bodyAndClose, bs.rules, make(lsVisiting))

	return terminals
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) produceNestContext(
	nestNode *syntaxa.Grammar[TToken, TNodeKind],
) TContext {
	edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		IsNest:    true,
		NestLabel: nestNode.GrammarLabel,
	}

	return bs.gen.config.contextProducer(edCtx)
}

func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) setupWrapperContext(
	s *EditorState[TObservation, TContext],
	nestCtx TContext,
	bodyTerminals []lsTerminal[TToken, TNodeKind],
	nestNode *syntaxa.Grammar[TToken, TNodeKind],
) *EditorState[TObservation, TContext] {
	s.Context = nestCtx
	contentName := s.ID + "_CONTENT"
	cs := &EditorState[TObservation, TContext]{ID: contentName, Label: contentName}
	s.ImmediatePushTarget = cs
	bs.states = append(bs.states, cs)

	lsPropagatePopOffset(bodyTerminals, 1)

	if len(bodyTerminals) > 0 {
		bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
			state:     cs,
			ctxKey:    "NEST_BODY_CONTENT:" + string(nestNode.GrammarLabel),
			terminals: bodyTerminals,
			nameHint:  nestNode.GrammarLabel,
		})
	}
	return s
}

func lsPropagatePopOffset[TToken, TNodeKind comparable](terminals []lsTerminal[TToken, TNodeKind], offset int) {
	for i := range terminals {
		terminals[i].popOffset = offset
	}
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTokenMap() {
	for _, rule := range e.lexingRuleset.GetRules() {
		e.tokenToRule[rule.Token] = rule
	}
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) tokenPriority(token TToken) int {
	if rule, ok := e.tokenToRule[token]; ok {
		return rule.Priority
	}
	return 0
}

func lsOuterFrameKind[TToken, TNodeKind comparable](term lsTerminal[TToken, TNodeKind]) (bool, bool) {
	if len(term.stack) == 0 {
		return false, false
	}
	outer := term.stack[len(term.stack)-1]
	if outer.repeatNode == nil {
		return false, false
	}
	if outer.repeatNode.Kind == syntaxa.GRepeat {
		return true, false
	}
	if outer.repeatNode.Kind == syntaxa.GOptional {
		return false, true
	}
	return false, false
}

func lsAllOptionalTerminals[TToken, TNodeKind comparable](terminals []lsTerminal[TToken, TNodeKind]) bool {
	if len(terminals) == 0 {
		return false
	}
	for _, t := range terminals {
		_, isOpt := lsOuterFrameKind(t)
		if !isOpt {
			return false
		}
	}
	return true
}

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getOrCreateDelimitedState(
	dp *DelimitedPayload[TObservation, TContext],
) *EditorState[TObservation, TContext] {
	if s, ok := e.delimitedStates[dp.StateLabel]; ok {
		return s
	}
	s := &EditorState[TObservation, TContext]{
		ID:      dp.StateLabel,
		Label:   dp.StateLabel,
		Context: dp.BodyContext,
		Transitions: []EditorTransition[TObservation, TContext]{
			{
				OnPattern:    dp.ClosePattern,
				MatchContext: dp.CloseContext,
				Operation:    STACK_POP,
				PopAmount:    1,
			},
		},
	}
	e.delimitedStates[dp.StateLabel] = s
	return s
}

func lsCollectGrammarTokens[TToken, TNodeKind comparable](
	grammars map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
) map[TToken]bool {
	seen := make(map[*syntaxa.Grammar[TToken, TNodeKind]]bool)
	result := make(map[TToken]bool)
	for _, g := range grammars {
		lsWalkGrammar(g, grammars, seen, result)
	}
	return result
}

func lsWalkGrammar[TToken, TNodeKind comparable](
	g *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	seen map[*syntaxa.Grammar[TToken, TNodeKind]]bool,
	tokens map[TToken]bool,
) {
	if g == nil || seen[g] {
		return
	}
	seen[g] = true

	switch g.Kind {
	case syntaxa.GToken:
		tokens[g.Token] = true
	case syntaxa.GNest:
		if g.OpenToken != nil {
			tokens[*g.OpenToken] = true
		}
		if g.CloseToken != nil {
			tokens[*g.CloseToken] = true
		}
	case syntaxa.GReference:
		target := g.ResolvedReference
		if target == nil {
			target = rules[g.ReferenceTarget]
		}
		lsWalkGrammar(target, rules, seen, tokens)
	}

	for _, child := range g.Children {
		lsWalkGrammar(child, rules, seen, tokens)
	}
}

func lsBuildAmbientTransitions[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	gen *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	grammars map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) []EditorTransition[TObservation, TContext] {
	grammarTokens := lsCollectGrammarTokens(grammars)

	type ruleEntry struct {
		rule     lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]
		priority int
	}
	var candidates []ruleEntry
	for _, rule := range gen.lexingRuleset.GetRules() {
		if !grammarTokens[rule.Token] {
			candidates = append(candidates, ruleEntry{rule: rule, priority: rule.Priority})
		}
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})

	var transitions []EditorTransition[TObservation, TContext]
	for _, entry := range candidates {
		transitions = append(transitions, buildAmbientTransition(gen, bs, entry.rule))
	}
	return transitions
}

func buildAmbientTransition[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	gen *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	rule lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole],
) EditorTransition[TObservation, TContext] {
	token := rule.Token
	edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &token}
	override, hasOverride := gen.config.overrideProducer(edCtx)
	matchCtx := gen.config.contextProducer(edCtx)

	var pat pattern.RegulaAST[TObservation]
	if hasOverride && override.Pattern != nil {
		pat = *override.Pattern
	} else {
		pat = rule.Pattern
	}

	if hasOverride && override.MatchContext != nil {
		matchCtx = *override.MatchContext
	}

	var captures map[int]TContext
	if hasOverride {
		captures = override.Captures
	}

	if hasOverride && override.ForeignPayload != nil {
		return EditorTransition[TObservation, TContext]{
			OnPattern:      pat,
			MatchContext:   matchCtx,
			Captures:       captures,
			Operation:      STACK_EMBED,
			ForeignPayload: override.ForeignPayload,
		}
	}

	if hasOverride && override.DelimitedPayload != nil {
		bodyState := gen.getOrCreateDelimitedState(override.DelimitedPayload)
		registerBodyStateIfNeeded(bs, bodyState)
		return EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    STACK_PUSH,
			Targets:      []*EditorState[TObservation, TContext]{bodyState},
		}
	}

	return EditorTransition[TObservation, TContext]{
		OnPattern:    pat,
		MatchContext: matchCtx,
		Captures:     captures,
		Operation:    STACK_NONE,
	}
}

func registerBodyStateIfNeeded[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	bodyState *EditorState[TObservation, TContext],
) {
	alreadyInStates := false
	for _, s := range bs.states {
		if s.Label == bodyState.Label {
			alreadyInStates = true
			break
		}
	}
	if !alreadyInStates {
		bs.states = append(bs.states, bodyState)
	}
}
