package editor

import (
	"autarch/pattern"
	"cmp"
	"fmt"
	"foundation/text"
	"lexarch"
	"memarch"
	"sort"
	"strings"
	"syntaxa"
)

const ROOT_LABEL = "root"

// lsMaxKeyDepth is the maximum stack depth encoded in a context key.
// Stack frames beyond this depth are omitted, which bounds context-key space
// for recursive grammars (e.g. Pratt parsers) at the cost of merging
// semantically-identical deep contexts — acceptable for syntax highlighting.
//
// A value of 8 was chosen empirically: the LangSpec grammar (101 rules, 4-level
// Pratt precedence tower) generates 231 distinct contexts at depth 8 and
// terminates in < 60 ms. Lowering this value merges more contexts (fewer states,
// less precise scoping); raising it increases state count and may cause the
// algorithm to diverge for highly-recursive grammars unless GNest boundaries
// are sufficient to break the recursion.
const lsMaxKeyDepth = 8

// ------------------------------------------------------------- PUBLIC TYPES

type EditorCtx[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable] struct {
	Token     *TToken
	TokenRole *TTokenRole
	// NodeKind is the parser node kind associated with this token, as declared via
	// the grammar's OutputNodeKind field (e.g. NodeParseRuleName for TokIdentifier
	// inside expectToken(NodeParseRuleName, TokIdentifier)). Nil when the token
	// has no named node binding (e.g. virtual/structural tokens).
	NodeKind *TNodeKind
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
	// nestContextProducer, when non-nil, is called for each GNest body state and
	// returns the TContext whose MetaScope (if any) is emitted as the Sublime
	// meta_scope for that context. This is the primary hook for adding a
	// semantically-named meta-scope to every delimited block (e.g. a PARSE block).
	nestContextProducer func(nestLabel syntaxa.GrammarLabel) TContext

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
	// HasFallthroughPop, when true, indicates that the Sublime context for this state
	// should include a lookahead-POP fallthrough rule ("(?=\S)" → pop: N) as its
	// last match rule. This is needed for afterNest continuation contexts whose optional
	// tail tokens (e.g. `;`) would otherwise leave the context permanently on the stack.
	// When FallthroughPopAmount is zero, a pop depth of 1 is used.
	HasFallthroughPop    bool
	FallthroughPopAmount int

	// ImmediatePushTarget, when non-nil, causes the syntax generator to emit a
	// zero-width "match: ''" → push: [ImmediatePushTarget] entry as the very first
	// transition (after any meta_scope). This is used by the nest-body wrapper
	// pattern: the wrapper state carries only the meta_scope and immediately pushes
	// the companion content state, which performs all actual token matching.
	// The wrapper state itself has no regular Transitions.
	ImmediatePushTarget *EditorState[TObservation, TContext]
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

// WithNestContextProducer sets a function that derives the TContext for each
// GNest body state from its GrammarLabel. Use this to assign a meta_scope to
// every delimited block in the generated Sublime syntax file.
func (c *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) WithNestContextProducer(
	fn func(nestLabel syntaxa.GrammarLabel) TContext,
) *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext] {
	c.nestContextProducer = fn
	return c
}


// lsStackEntry mirrors SBNF's StackEntry: one frame in a terminal's continuation stack.
// Stack grows outermost-last: index 0 = innermost, last = outermost.
type lsStackEntry[TToken, TNodeKind comparable] struct {
	isRepetition bool
	label        syntaxa.GrammarLabel
	repeatNode   *syntaxa.Grammar[TToken, TNodeKind]
	remaining    []*syntaxa.Grammar[TToken, TNodeKind]
}

// lsTerminal mirrors SBNF's Terminal: the first-token of a grammar position
// together with its full continuation state (remaining + caller stack).
//
// nestNode, when non-nil, marks this terminal as the open-token of a GNest.
// The transition builder will generate a push to the nest's body context
// rather than inlining the body into the continuation stack, which prevents
// recursive grammars (e.g. expressions inside parentheses) from blowing up.
//
// nodeKind, when non-nil, carries the OutputNodeKind of the grammar node that
// produced this terminal (e.g. NodeParseRuleName for TokIdentifier in
// expectToken(NodeParseRuleName, TokIdentifier)). It is used by the context
// producer to apply node-specific scopes in the generated syntax.
//
// popOffset accumulates an extra pop depth when this terminal lives inside the
// companion content state of a meta-scope wrapper nest body. A value of 1 means
// "pop one extra level" so that the close-token pop removes both the companion
// content state AND the wrapper state from the Sublime stack. The offset
// propagates through lsAdvanceTerminal so all states in the chain inherit it.
type lsTerminal[TToken, TNodeKind comparable] struct {
	token     TToken
	remaining []*syntaxa.Grammar[TToken, TNodeKind]
	stack     []lsStackEntry[TToken, TNodeKind]
	nestNode  *syntaxa.Grammar[TToken, TNodeKind] // non-nil → open token of this GNest
	nodeKind  *TNodeKind                          // non-nil → OutputNodeKind from the GToken node
	popOffset int                                 // extra pop depth for wrapped nest body chains
}

// getLastRemaining returns a pointer to the remaining of the outermost frame.
// Mirrors SBNF's Terminal::get_last_remaining.
func (t *lsTerminal[TToken, TNodeKind]) getLastRemaining() *[]*syntaxa.Grammar[TToken, TNodeKind] {
	if len(t.stack) == 0 {
		return &t.remaining
	}
	return &t.stack[len(t.stack)-1].remaining
}

type lsVisiting map[syntaxa.GrammarLabel]bool

// ------------------------------------------------------------- SBNF LOOKAHEAD

// lsLookahead computes the set of first terminals reachable from node.
// GNest nodes are treated as push boundaries: only their open token is returned,
// with nestNode set, preventing recursive body expansion.
func lsLookahead[TToken, TNodeKind comparable](
	node *syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) (terms []lsTerminal[TToken, TNodeKind], nullable bool) {
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
		var all []lsTerminal[TToken, TNodeKind]
		anyNull := false
		for _, child := range node.Children {
			ts, n := lsLookahead(child, rules, visiting)
			all = append(all, ts...)
			anyNull = anyNull || n
		}
		return all, anyNull

	case syntaxa.GRepeat:
		child := node.Children[0]
		ts, _ := lsLookahead(child, rules, visiting)
		for i := range ts {
			ts[i].stack = append(ts[i].stack, lsStackEntry[TToken, TNodeKind]{
				isRepetition: true,
				repeatNode:   node,
				// Carry the node label so lsOwningRule can return a semantic name even
				// when no GReference (Variable) frame wraps the repetition.
				label: node.GrammarLabel,
			})
		}
		return ts, node.Min == 0

	case syntaxa.GOptional:
		child := node.Children[0]
		ts, _ := lsLookahead(child, rules, visiting)
		for i := range ts {
			ts[i].stack = append(ts[i].stack, lsStackEntry[TToken, TNodeKind]{
				isRepetition: true,
				repeatNode:   node,
				label:        node.GrammarLabel,
			})
		}
		return ts, true

	case syntaxa.GReference:
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

	case syntaxa.GNest:
		// Treat the GNest as a push boundary: return only the open token,
		// with nestNode set so buildTransition knows to create a body context.
		// This prevents recursive expression grammars from creating infinite
		// continuation chains through the nest body.
		t := lsTerminal[TToken, TNodeKind]{
			token:    *node.OpenToken,
			nestNode: node,
		}
		return []lsTerminal[TToken, TNodeKind]{t}, false
	}

	return nil, false
}

// lsLookaheadConcat computes the lookahead of a sequence of grammar nodes.
// Mirrors SBNF's lookahead_concatenation.
func lsLookaheadConcat[TToken, TNodeKind comparable](
	nodes []*syntaxa.Grammar[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	visiting lsVisiting,
) (terms []lsTerminal[TToken, TNodeKind], nullable bool) {
	nullable = true
	for i, node := range nodes {
		ts, n := lsLookahead(node, rules, visiting)

		suffix := nodes[i+1:]
		for j := range ts {
			// nestNode terminals carry the outer continuation in their remaining
			// so buildTransition knows what to push after the nest body pops.
			last := ts[j].getLastRemaining()
			extended := make([]*syntaxa.Grammar[TToken, TNodeKind], len(*last)+len(suffix))
			copy(extended, *last)
			copy(extended[len(*last):], suffix)
			*last = extended
		}

		terms = append(terms, ts...)

		if !n {
			nullable = false
			return
		}
	}
	return
}

// ------------------------------------------------------------- SBNF ADVANCE

// lsAdvanceTerminal computes the next lookahead after terminal t has been matched.
// Mirrors SBNF's advance_terminal.
//
// For repetition frames (outermost frame is a Repetition), the caller strips that
// frame first (see buildTransition). This function only sees the stripped terminal.
//
// Returns nil if nothing follows (caller should emit STACK_POP).
func lsAdvanceTerminal[TToken, TNodeKind comparable](
	term lsTerminal[TToken, TNodeKind],
	rules map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
) []lsTerminal[TToken, TNodeKind] {
	levels := len(term.stack) + 1

	var result []lsTerminal[TToken, TNodeKind]

	for i := 0; i < levels; i++ {
		var remaining []*syntaxa.Grammar[TToken, TNodeKind]
		var isRep bool
		var repNode *syntaxa.Grammar[TToken, TNodeKind]

		if i == 0 {
			remaining = term.remaining
		} else {
			entry := term.stack[i-1]
			remaining = entry.remaining
			isRep = entry.isRepetition
			repNode = entry.repeatNode
		}

		if len(remaining) == 0 && !isRep {
			continue
		}
		if isRep && len(remaining) == 0 {
			continue
		}

		visiting := make(lsVisiting)

		var la []lsTerminal[TToken, TNodeKind]
		var nullable bool

		if isRep {
			loopNodes := make([]*syntaxa.Grammar[TToken, TNodeKind], 0, 1+len(remaining))
			loopNodes = append(loopNodes, repNode)
			loopNodes = append(loopNodes, remaining...)
			la, nullable = lsLookaheadConcat(loopNodes, rules, visiting)
		} else {
			la, nullable = lsLookaheadConcat(remaining, rules, visiting)
		}

		outerStack := term.stack[i:]
		for j := range la {
			newStack := make([]lsStackEntry[TToken, TNodeKind], len(la[j].stack)+len(outerStack))
			copy(newStack, la[j].stack)
			copy(newStack[len(la[j].stack):], outerStack)
			la[j].stack = newStack
		}

		result = append(result, la...)

		if !nullable {
			break
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// ------------------------------------------------------------- CONTEXT KEY

// lsLookaheadKey produces a stable canonical key for a lookahead set.
// Stack frames beyond lsMaxKeyDepth are omitted so that recursive grammars
// converge (context count is bounded) rather than diverging.
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
		// Fall through to encode the continuation (remaining + stack) so that
		// different call-sites for the same nest token (e.g. `(` used inside
		// a Pratt BP-10 context vs. a BP-30 context) produce distinct keys and
		// are not incorrectly merged into one shared context.
	}
	sb.WriteString(":(")
	sb.WriteString(lsGrammarNodesKey(t.remaining, tokenFmt))
	sb.WriteString(")")

	depth := len(t.stack)
	if depth > lsMaxKeyDepth {
		depth = lsMaxKeyDepth
	}
	for _, e := range t.stack[:depth] {
		if e.isRepetition {
			sb.WriteString(":R[")
			sb.WriteString(lsGrammarNodesKey(e.remaining, tokenFmt))
			sb.WriteString("]")
		} else {
			sb.WriteString(":V(")
			sb.WriteString(string(e.label))
			sb.WriteString(",")
			sb.WriteString(lsGrammarNodesKey(e.remaining, tokenFmt))
			sb.WriteString(")")
		}
	}
	if len(t.stack) > lsMaxKeyDepth {
		sb.WriteString(":...")
	}
	// Include popOffset so that states inside a wrapped nest body companion
	// (popOffset > 0) are never shared with states outside it (popOffset = 0).
	// Without this, two contexts could end up using the same state but one
	// needs pop:2 while the other needs pop:1.
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

// lsOwningRule returns the grammar label that best describes the context of terminal t.
//
// Priority:
//  1. Innermost non-Repetition frame (a GReference Variable frame) – most specific.
//  2. Innermost Repetition frame that carries a label (a GOptional/GRepeat with a
//     GrammarLabel, e.g. "PRAGMA SECTION") – covers grammars that embed rules directly
//     (without GReference wrappers) by labelling the enclosing Optional/Repeat node.
//  3. "anon" – no semantic label found; caller should fall back to the nameHint.
func lsOwningRule[TToken, TNodeKind comparable](term lsTerminal[TToken, TNodeKind]) syntaxa.GrammarLabel {
	// First pass: innermost non-Repetition (Variable) frame.
	for i := 0; i < len(term.stack); i++ {
		if !term.stack[i].isRepetition && term.stack[i].label != "" {
			return term.stack[i].label
		}
	}
	// Second pass: innermost Repetition frame with a non-empty label.
	for i := 0; i < len(term.stack); i++ {
		if term.stack[i].isRepetition && term.stack[i].label != "" {
			return term.stack[i].label
		}
	}
	return "anon"
}

// ------------------------------------------------------------- GENERATOR

type editorIRGenerator[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any] struct {
	lexingRuleset  *lexarch.LexingRuleset[TObservation, TToken, TTokenRole]
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	config         *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]
	sanitizer      *text.Sanitizer

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
	// nameHint is the grammar label used as a fallback name when lsOwningRule returns "anon".
	// It is set to the enclosing GNest's GrammarLabel so that states inside a nest body
	// get readable names (e.g. "HEADER__0") instead of "anon__N".
	nameHint syntaxa.GrammarLabel
}

// EditorIRCreate generates an EditorIR from a grammar package using the SBNF algorithm.
//
// Algorithm overview (faithful adaptation of SBNF's codegen pipeline):
//
//  1. Compute the first-token lookahead set (with continuation stacks) for the entry rule.
//  2. For each terminal in a context, compute advance_terminal to find the next state.
//  3. GRepeat (ZeroOrMore/Repeat) frames use pop=0: the loop context stays on the Sublime
//     stack so later iterations can occur naturally.
//     - Emit STACK_NONE when nothing follows the loop body token.
//     - Emit STACK_PUSH when a continuation context is needed.
//  4. GOptional frames use pop=1: optional elements advance without keeping the current
//     context, which prevents dangling continuation contexts.
//     - Emit STACK_POP when nothing follows the optional.
//     - Emit STACK_SET when a continuation context is needed.
//  5. Non-repetition continuations: emit SET (pop=1 + push continuation).
//     Emit POP when nothing follows.
//  6. GNest push-boundary: open tokens carry nestNode; buildTransition creates a
//     dedicated body context and emits PUSH, preventing recursive expansion.
//  7. All contexts are memoised by their lookahead key (bounded by lsMaxKeyDepth).
//  8. Terminals within a context are sorted by token lexer priority (descending)
//     so higher-priority tokens (e.g. keywords) always appear before lower-priority
//     tokens (e.g. identifiers) in the generated Sublime syntax file.
func EditorIRCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	lexingRuleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	config *EditorIRConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) (*EditorIR[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext], error) {

	gen := &editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		lexingRuleset:   lexingRuleset,
		grammarPackage:  grammarPackage,
		config:          config,
		sanitizer:       text.NewIdentifierSanitizer(),
		tokenToRule:     make(map[TToken]lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]),
		delimitedStates: make(map[string]*EditorState[TObservation, TContext]),
	}
	gen.buildTokenMap()

	bs := &lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]{
		gen:          gen,
		rules:        grammarPackage.Grammars,
		contextByKey: make(map[string]*EditorState[TObservation, TContext]),
		counters:     make(map[string]int),
	}

	entryLabel := grammarPackage.EntryRule
	entryRule := grammarPackage.Grammars[entryLabel]
	if entryRule == nil {
		return nil, fmt.Errorf("EditorIRCreate: entry rule %q not found in grammar package", entryLabel)
	}

	entryTerminals, _ := lsLookahead[TToken, TNodeKind](entryRule, bs.rules, make(lsVisiting))
	rootState := &EditorState[TObservation, TContext]{ID: ROOT_LABEL, Label: ROOT_LABEL}
	rootKey := lsLookaheadKey(entryTerminals, config.tokenFormatter)
	bs.contextByKey[rootKey] = rootState
	bs.states = append(bs.states, rootState)
	bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
		state:     rootState,
		ctxKey:    rootKey,
		terminals: entryTerminals,
	})

	for len(bs.queue) > 0 {
		pending := bs.queue[0]
		bs.queue = bs.queue[1:]
		bs.processContext(pending)
	}

	// Compute ambient transitions: tokens that appear in the lexing ruleset but
	// NOT in any grammar rule become prototype-level rules in Sublime Text. This
	// causes them (e.g. whitespace, line/block comments) to be highlighted in
	// every context without having to be explicitly listed in each one.
	// This must run before allStates is built so that any delimited body states
	// created for ambient transitions (e.g. block_comment_inner) are included.
	ambientTransitions := lsBuildAmbientTransitions(gen, grammarPackage.Grammars, bs)

	// Collect all states. bs.states is the authoritative list; it now includes
	// any delimited body states registered during ambient transition building.
	// States created by inline grammar-derived buildTransition calls for
	// DelimitedPayload overrides are also harvested from gen.delimitedStates to
	// ensure they appear in the output even when the grammar references them.
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
		// If this state is a delimited body state we already registered it via
		// bs.states; remove it from the pending harvest to avoid duplication.
		delete(delimitedStateKeys, s.Label)
	}
	// Add any remaining delimited states not yet in bs.states (grammar-derived).
	for label := range delimitedStateKeys {
		if s, ok := gen.delimitedStates[label]; ok {
			allStates = append(allStates, *s)
		}
	}

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

// processContext generates all transitions for one pending context.
// Terminals are sorted by token lexer priority (descending) before building
// transitions, ensuring higher-priority tokens (e.g. keyword literals with
// priority 2) appear before lower-priority tokens (e.g. the identifier regex
// with priority 1) in the generated Sublime syntax file.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) processContext(
	pending lsPendingCtx[TObservation, TToken, TNodeKind, TContext],
) {
	// Sort terminals by priority descending so higher-priority tokens (keywords)
	// appear before lower-priority tokens (identifiers) in the generated context.
	// sort.SliceStable preserves the grammar-order tie-breaking between equal priorities.
	sort.SliceStable(pending.terminals, func(i, j int) bool {
		return bs.gen.tokenPriority(pending.terminals[i].token) >
			bs.gen.tokenPriority(pending.terminals[j].token)
	})

	for _, term := range pending.terminals {
		tr := bs.buildTransition(term, pending.nameHint)
		if tr == nil {
			continue
		}
		pending.state.Transitions = append(pending.state.Transitions, *tr)
	}
}

// getOrCreateContext returns the EditorState for the given lookahead set,
// creating and enqueuing it if it does not yet exist.
//
// nameHint provides a fallback grammar label when lsOwningRule would return "anon"
// (i.e. the terminal set has no Variable-frame labels). It is propagated to the
// queued pending context so child states inherit the same enclosing-rule prefix.
//
// A context is automatically marked HasFallthroughPop when ALL of its terminals
// have a GOptional outermost rep frame. Such contexts consist entirely of optional
// elements that may be absent from the input; without a fallthrough pop the context
// would be left permanently on the Sublime stack when none of its rules fire.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getOrCreateContext(
	terminals []lsTerminal[TToken, TNodeKind],
	ownerLabel syntaxa.GrammarLabel,
	nameHint syntaxa.GrammarLabel,
) *EditorState[TObservation, TContext] {
	key := lsLookaheadKey(terminals, bs.gen.config.tokenFormatter)
	if s, ok := bs.contextByKey[key]; ok {
		return s
	}

	// Use ownerLabel when meaningful; fall back to the propagated hint otherwise.
	// "anon" is the sentinel returned by lsOwningRule when no Variable frame exists;
	// "" occurs when ownerLabel was "anon" and a prior nameHint was also empty.
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

	// Inherit the effective label as the hint for all child states of this context.
	childHint := effectiveLabel

	s := &EditorState[TObservation, TContext]{ID: name, Label: name}

	// A context whose entire terminal set consists of GOptional-typed outermost
	// frames needs a fallthrough-pop so Sublime can escape the context when none
	// of the optional elements are present in the input. (GRepeat outermost frames
	// represent ZeroOrMore loops that must stay on the stack; they do not need the
	// fallthrough.)
	if lsAllOptionalTerminals(terminals) {
		s.HasFallthroughPop = true
		// Propagate the pop depth from the terminals so that wrapped nest body
		// chains pop both the companion content state and the wrapper state.
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
		nameHint:  childHint,
	})
	return s
}

// buildTransition constructs the EditorTransition for a single terminal.
//
// GRepeat vs GOptional pop=0 / pop=1 mapping to EditorTransition:
//   - GRepeat (ZeroOrMore/Repeat) outermost frame → pop=0:
//     The loop context stays on the Sublime stack for subsequent iterations.
//     - pop=0 + no contexts  → STACK_NONE  (loop; current context stays)
//     - pop=0 + contexts     → STACK_PUSH  (push continuation, loop stays)
//   - GOptional outermost frame → pop=1:
//     Optional elements must not keep their pushed context alive indefinitely.
//     - pop=1 + no contexts  → STACK_POP(1)
//     - pop=1 + contexts     → STACK_SET
//   - No rep frame (non-repetition) → pop=1:
//     - pop=1 + no contexts  → STACK_POP(1)
//     - pop=1 + contexts     → STACK_SET
//
// When term.popOffset > 0 (inside a wrapped nest body's companion content state),
// all STACK_POP operations use PopAmount = 1 + term.popOffset so that they exit
// both the companion content state and the wrapper state. The popOffset is
// propagated to continuation terminals via lsAdvanceTerminal so the entire chain
// within the companion state consistently uses the extra pop depth.
//
// GNest push-boundary:
//   - isZeroOrMore=false (linear sequence) → STACK_SET [afterNest, body]:
//     the current continuation context is replaced so it cannot linger on the stack
//     after the nest closes.
//   - isZeroOrMore=true (inside a ZeroOrMore/Repeat body) → STACK_PUSH [afterNest, body]:
//     the loop context must stay so the next iteration can fire.
//
// nameHint is a fallback label used when lsOwningRule returns "anon"; it is the
// GrammarLabel of the nearest enclosing GNest or named rule.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTransition(
	term lsTerminal[TToken, TNodeKind],
	nameHint syntaxa.GrammarLabel,
) *EditorTransition[TObservation, TContext] {

	cfg := bs.gen.config
	tokenRule, hasTokenRule := bs.gen.tokenToRule[term.token]

	edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &term.token, NodeKind: term.nodeKind}
	override, hasOverride := cfg.overrideProducer(edCtx)
	matchCtx := cfg.contextProducer(edCtx)

	var pat pattern.RegulaAST[TObservation]
	switch {
	case hasOverride && override.Pattern != nil:
		pat = *override.Pattern
	case hasTokenRule:
		pat = tokenRule.Pattern
	default:
		return nil // no pattern available
	}

	if hasOverride && override.MatchContext != nil {
		matchCtx = *override.MatchContext
	}

	var captures map[int]TContext
	if hasOverride {
		captures = override.Captures
	}

	// ForeignPayload: embed external syntax machine (e.g. source.regexp).
	if hasOverride && override.ForeignPayload != nil {
		return &EditorTransition[TObservation, TContext]{
			OnPattern:      pat,
			MatchContext:   matchCtx,
			Captures:       captures,
			Operation:      STACK_EMBED,
			ForeignPayload: override.ForeignPayload,
		}
	}

	// DelimitedPayload: open token → push body state; body state handles close → pop.
	if hasOverride && override.DelimitedPayload != nil {
		dp := override.DelimitedPayload
		bodyState := bs.gen.getOrCreateDelimitedState(dp)
		return &EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    STACK_PUSH,
			Targets:      []*EditorState[TObservation, TContext]{bodyState},
		}
	}

	// Determine the outermost stack frame's repetition kind.
	//
	// - isZeroOrMore: outermost frame is GRepeat (ZeroOrMore/Repeat) → pop=0
	//   The loop context stays on the Sublime stack for subsequent iterations.
	// - isOptional:   outermost frame is GOptional → pop=1
	//   Optional elements are treated as single-use; they must advance or pop.
	// - neither:      no rep frame → pop=1 (non-repetition continuation).
	isZeroOrMore, _ := lsOuterFrameKind(term)

	// GNest push-boundary: open token → push an independent body context.
	// The body is computed without the outer continuation stack so that recursive
	// expression grammars (e.g. Pratt parsers) don't blow up the continuation chain.
	if term.nestNode != nil {
		bodyState := bs.getOrCreateNestBodyContext(term.nestNode)
		ownerLabel2 := lsOwningRule(term)
		// nestHint ensures child states are named after the enclosing nest even when
		// the outer terminal has no Variable frame (ownerLabel2 == "anon").
		nestHint := syntaxa.GrammarLabel(string(term.nestNode.GrammarLabel))
		// Compute the continuation after the nest closes (strips nestNode to let
		// advance see the remaining/stack normally).
		noNest := lsTerminal[TToken, TNodeKind]{token: term.token, remaining: term.remaining, stack: term.stack}
		advTerminals := lsAdvanceTerminal(noNest, bs.rules)
		// Propagate the pop offset into the afterNest continuation so that all
		// states derived from it also use the correct pop depth.
		lsPropagatePopOffset(advTerminals, term.popOffset)

		// Choose PUSH (loop body) or SET (linear sequence) for the nest open token.
		// SET replaces the current continuation context so it cannot accumulate on the
		// Sublime stack (fixing the "orphan anon__N context after {}" bug).
		nestOp := STACK_SET
		if isZeroOrMore {
			nestOp = STACK_PUSH
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
		// Mark afterNest as needing a fallthrough POP so that optional tail tokens
		// (e.g. the ';' after '}') do not leave the context permanently on the stack.
		afterNest := bs.getOrCreateContext(advTerminals, ownerLabel2, nestHint)
		afterNest.HasFallthroughPop = true
		// If the continuation is inside a wrapped nest body, ensure the fallthrough
		// pop depth is also updated to exit both the companion state and the wrapper.
		if term.popOffset > 0 && afterNest.FallthroughPopAmount == 0 {
			afterNest.FallthroughPopAmount = 1 + term.popOffset
		}
		return &EditorTransition[TObservation, TContext]{
			OnPattern:    pat,
			MatchContext: matchCtx,
			Captures:     captures,
			Operation:    nestOp,
			// afterNest pushed deeper, bodyState on top (processed first).
			Targets: []*EditorState[TObservation, TContext]{afterNest, bodyState},
		}
	}

	// SBNF repetition / continuation logic.
	ownerLabel := lsOwningRule(term)

	var advTerminals []lsTerminal[TToken, TNodeKind]

	if isZeroOrMore {
		// GRepeat (pop=0): advance without the outermost Repetition frame
		// (iteration continuation only).  The loop context stays on the Sublime
		// stack via PUSH so further iterations are visible.
		stripped := lsTerminal[TToken, TNodeKind]{
			token:     term.token,
			remaining: term.remaining,
			stack:     term.stack[:len(term.stack)-1],
		}
		advTerminals = lsAdvanceTerminal(stripped, bs.rules)
	} else {
		// GOptional (pop=1) or non-rep: advance including the full stack
		// so the continuation knows what follows the optional element.
		advTerminals = lsAdvanceTerminal(term, bs.rules)
	}
	// Propagate the pop offset so continuation states use the same pop depth.
	lsPropagatePopOffset(advTerminals, term.popOffset)

	if isZeroOrMore {
		// pop=0: keep the loop context (current state) on the Sublime stack.
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

	// pop=1+popOffset (GOptional or non-rep): replace current context with continuation, or pop.
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

// getOrCreateNestBodyContext returns the EditorState for the interior of a GNest.
// The context is keyed by the nest node's GrammarLabel so that the same nest
// reused at different call sites always maps to the same Sublime context
// (allowing correct push/pop for balanced delimiters at any nesting depth).
//
// If config.nestContextProducer is set and returns a non-zero context, the
// function implements the "meta-scope wrapper" pattern to ensure the meta_scope
// persists across all tokens inside the nest body:
//
//  1. A wrapper state (with the meta_scope context) is created and keyed by the
//     nest label.  It carries no regular transitions; instead its ImmediatePushTarget
//     is set to a companion content state.
//  2. A companion content state (no meta_scope) holds all actual terminal
//     transitions.  Its body terminals carry popOffset=1 so that every STACK_POP
//     they generate uses pop:2, exiting both the companion and the wrapper.
//
// Without the wrapper, a nest body context that uses set: for its first transition
// would immediately replace itself on the Sublime stack, losing the meta_scope.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getOrCreateNestBodyContext(
	nestNode *syntaxa.Grammar[TToken, TNodeKind],
) *EditorState[TObservation, TContext] {
	nestKey := "NEST_BODY:" + string(nestNode.GrammarLabel)
	if s, ok := bs.contextByKey[nestKey]; ok {
		return s
	}

	name := bs.gen.sanitizer.Sanitize(string(nestNode.GrammarLabel))
	if name == "" {
		name = "nest_body"
	}

	// Compute body lookahead FRESH (no outer continuation), appending the close
	// token so the natural end of the body chain emits a POP for the close token.
	// This prevents recursive expression grammars from expanding infinitely while
	// ensuring the close token is correctly handled at the end of the sequence.
	closeTokenNode := &syntaxa.Grammar[TToken, TNodeKind]{Kind: syntaxa.GToken, Token: *nestNode.CloseToken}
	bodyAndClose := []*syntaxa.Grammar[TToken, TNodeKind]{nestNode.Children[0], closeTokenNode}
	bodyTerminals, _ := lsLookaheadConcat[TToken, TNodeKind](bodyAndClose, bs.rules, make(lsVisiting))

	// Determine whether the nest body produces a non-empty meta-scope context.
	// If yes, we use the wrapper pattern so the meta_scope persists across set:
	// transitions inside the body.
	var nestCtx TContext
	var useWrapper bool
	if bs.gen.config.nestContextProducer != nil {
		nestCtx = bs.gen.config.nestContextProducer(nestNode.GrammarLabel)
		var zero TContext
		useWrapper = !bs.gen.config.contextsEqual(nestCtx, zero)
	}

	s := &EditorState[TObservation, TContext]{ID: name, Label: name}
	// Register BEFORE computing body so recursive nests find this state and reuse it.
	bs.contextByKey[nestKey] = s
	bs.states = append(bs.states, s)

	if useWrapper {
		// Wrapper pattern: s carries only the meta_scope; a companion content state
		// cs holds all actual transitions.  Body terminals get popOffset=1 so that
		// every STACK_POP they (or their descendants) generate pops both cs and s.
		s.Context = nestCtx

		contentName := name + "_CONTENT"
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

	// No wrapper needed: nestContextProducer is nil or returned an empty (zero)
	// context for this nest, so there is no meta_scope to keep alive. Process the
	// body terminals directly in s without any wrapper indirection.
	if bs.gen.config.nestContextProducer != nil {
		s.Context = nestCtx
	}

	if len(bodyTerminals) > 0 {
		bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
			state:     s,
			ctxKey:    nestKey,
			terminals: bodyTerminals,
			// Propagate the nest label as the naming hint for all child states in the body,
			// giving them readable names like "HEADER__0" instead of "anon__N".
			nameHint: nestNode.GrammarLabel,
		})
	}

	return s
}

// ------------------------------------------------------------- HELPERS

// lsPropagatePopOffset sets the popOffset field on every terminal in the slice
// to the given value.  This is used to propagate the extra pop depth from a
// wrapped nest body's companion content state into all continuation terminals
// so that every STACK_POP in the chain correctly exits both the companion state
// and the meta-scope wrapper state.
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

// tokenPriority returns the lexer rule priority for the given token.
// Terminals with higher priority (e.g. keyword literals at priority 2) are
// placed before lower-priority terminals (e.g. the identifier regex at priority 1)
// in the generated Sublime context so that keywords are not shadowed.
func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) tokenPriority(token TToken) int {
	if rule, ok := e.tokenToRule[token]; ok {
		return rule.Priority
	}
	return 0
}

// lsOuterFrameKind inspects the outermost stack frame of a terminal and reports
// whether it represents a ZeroOrMore/Repeat loop (isZeroOrMore) or a single
// optional occurrence (isOptional).  Both are false when the terminal carries no
// repetition frame (non-repetition continuation).
//
// The distinction drives pop=0 vs pop=1 in buildTransition:
//   - GRepeat  → pop=0: loop context must stay on the Sublime stack.
//   - GOptional → pop=1: optional element must not keep a dangling context.
//
// Note: lsLookahead marks GOptional stack frames with isRepetition=true (consistent
// with SBNF's internal model that treats optional and repeat uniformly as "rep frames"
// for purposes of lsOwningRule and lsLookaheadKey). The Kind-based check here is the
// correct place to distinguish optional from loop semantics.
func lsOuterFrameKind[TToken, TNodeKind comparable](term lsTerminal[TToken, TNodeKind]) (isZeroOrMore, isOptional bool) {
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

// lsAllOptionalTerminals reports whether every terminal in the set has a GOptional
// outermost frame. Such a set means the context consists entirely of optional
// elements; if none of them fire, Sublime would be stuck unless a fallthrough
// pop is present.
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

// getOrCreateDelimitedState returns (creating if needed) the EditorState
// for the interior of an override-driven delimited region (e.g. block comment).
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

// ------------------------------------------------------------- AMBIENT TRANSITIONS

// lsCollectGrammarTokens walks all grammars in the grammar map and returns the
// set of token types that are explicitly referenced by GToken nodes or as the
// open/close tokens of GNest nodes. This set is used to identify which lexer
// rules are "ambient" (not part of the grammar) and should be placed in the
// Sublime Text prototype context so they fire everywhere.
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

// lsWalkGrammar recursively walks a grammar node and its children, collecting
// all token types into the `tokens` map. Pointer-based cycle detection via
// `seen` prevents infinite loops in recursive grammars.
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

// lsBuildAmbientTransitions constructs the EditorTransition list for lexer rules
// that do not appear in any grammar rule. These "ambient" rules (e.g. whitespace,
// line comments, block comments) are placed in Sublime's prototype context so
// they are recognised in every highlighting context.
//
// Transitions are built using the same override/context producer pipeline as
// normal grammar-derived transitions so that, for example, block comments
// correctly produce a delimited push region and line comments produce captures.
// Ambient transitions are sorted by lexer rule priority (descending) so higher-
// priority ambient tokens shadow lower-priority ones when patterns overlap.
func lsBuildAmbientTransitions[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState, TNodeKind comparable, TContext any](
	gen *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
	grammars map[syntaxa.GrammarLabel]*syntaxa.Grammar[TToken, TNodeKind],
	bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext],
) []EditorTransition[TObservation, TContext] {

	grammarTokens := lsCollectGrammarTokens(grammars)

	// Gather rules for ambient tokens, preserving the original order for stability.
	type ruleEntry struct {
		rule     lexarch.LexerRuleReadOnly[TObservation, TToken, TTokenRole]
		priority int
	}
	var candidates []ruleEntry
	for _, rule := range gen.lexingRuleset.GetRules() {
		if grammarTokens[rule.Token] {
			continue
		}
		candidates = append(candidates, ruleEntry{rule: rule, priority: rule.Priority})
	}

	// Sort by priority descending (highest priority first), stable for equal priorities.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].priority > candidates[j].priority
	})

	var transitions []EditorTransition[TObservation, TContext]
	for _, entry := range candidates {
		token := entry.rule.Token
		edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &token}
		override, hasOverride := gen.config.overrideProducer(edCtx)
		matchCtx := gen.config.contextProducer(edCtx)

		var pat pattern.RegulaAST[TObservation]
		if hasOverride && override.Pattern != nil {
			pat = *override.Pattern
		} else {
			pat = entry.rule.Pattern
		}

		if hasOverride && override.MatchContext != nil {
			matchCtx = *override.MatchContext
		}

		var captures map[int]TContext
		if hasOverride {
			captures = override.Captures
		}

		var tr EditorTransition[TObservation, TContext]

		switch {
		case hasOverride && override.ForeignPayload != nil:
			tr = EditorTransition[TObservation, TContext]{
				OnPattern:      pat,
				MatchContext:   matchCtx,
				Captures:       captures,
				Operation:      STACK_EMBED,
				ForeignPayload: override.ForeignPayload,
			}
		case hasOverride && override.DelimitedPayload != nil:
			bodyState := gen.getOrCreateDelimitedState(override.DelimitedPayload)
			// Register the body state in bs.states so it is included in the
			// generated output. Guard against duplicates in case
			// getOrCreateDelimitedState was already called from a prior
			// grammar-derived buildTransition for the same token.
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
			tr = EditorTransition[TObservation, TContext]{
				OnPattern:    pat,
				MatchContext: matchCtx,
				Captures:     captures,
				Operation:    STACK_PUSH,
				Targets:      []*EditorState[TObservation, TContext]{bodyState},
			}
		default:
			tr = EditorTransition[TObservation, TContext]{
				OnPattern:    pat,
				MatchContext: matchCtx,
				Captures:     captures,
				Operation:    STACK_NONE,
			}
		}

		transitions = append(transitions, tr)
	}

	return transitions
}
