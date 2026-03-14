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
STACK_NONE  StackOperation = iota
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

// ------------------------------------------------------------- SBNF LOOKAHEAD TYPES

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
type lsTerminal[TToken, TNodeKind comparable] struct {
token     TToken
remaining []*syntaxa.Grammar[TToken, TNodeKind]
stack     []lsStackEntry[TToken, TNodeKind]
nestNode  *syntaxa.Grammar[TToken, TNodeKind] // non-nil → open token of this GNest
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
return []lsTerminal[TToken, TNodeKind]{{token: node.Token}}, false

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
return sb.String()
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
if !term.stack[i].isRepetition {
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
}

// EditorIRCreate generates an EditorIR from a grammar package using the SBNF algorithm.
//
// Algorithm overview (faithful adaptation of SBNF's codegen pipeline):
//
//  1. Compute the first-token lookahead set (with continuation stacks) for the entry rule.
//  2. For each terminal in a context, compute advance_terminal to find the next state.
//  3. Repetition frames (outermost stack frame is GRepeat/GOptional):
//     - Strip the rep frame; advance covers only the current iteration.
//     - Emit PUSH (pop=0): the loop context stays on the Sublime stack so later
//       iterations can occur naturally.
//  4. Non-repetition continuations: emit SET (pop=1 + push continuation).
//     Emit POP when nothing follows.
//  5. GNest push-boundary: open tokens carry nestNode; buildTransition creates a
//     dedicated body context and emits PUSH, preventing recursive expansion.
//  6. All contexts are memoised by their lookahead key (bounded by lsMaxKeyDepth).
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

allStates := make([]EditorState[TObservation, TContext], len(bs.states))
var rootOut EditorState[TObservation, TContext]
for i, s := range bs.states {
allStates[i] = *s
if s.Label == ROOT_LABEL {
rootOut = *s
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
AmbientTransitions: nil,
},
}, nil
}

// processContext generates all transitions for one pending context.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) processContext(
pending lsPendingCtx[TObservation, TToken, TNodeKind, TContext],
) {
for _, term := range pending.terminals {
tr := bs.buildTransition(term, pending.ctxKey)
if tr == nil {
continue
}
pending.state.Transitions = append(pending.state.Transitions, *tr)
}
}

// getOrCreateContext returns the EditorState for the given lookahead set,
// creating and enqueuing it if it does not yet exist.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) getOrCreateContext(
terminals []lsTerminal[TToken, TNodeKind],
ownerLabel syntaxa.GrammarLabel,
) *EditorState[TObservation, TContext] {
key := lsLookaheadKey(terminals, bs.gen.config.tokenFormatter)
if s, ok := bs.contextByKey[key]; ok {
return s
}

base := bs.gen.sanitizer.Sanitize(string(ownerLabel))
if base == "" {
base = "ctx"
}
n := bs.counters[base]
bs.counters[base] = n + 1
name := fmt.Sprintf("%s__%d", base, n)

s := &EditorState[TObservation, TContext]{ID: name, Label: name}
bs.contextByKey[key] = s
bs.states = append(bs.states, s)
bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
state:     s,
ctxKey:    key,
terminals: terminals,
})
return s
}

// buildTransition constructs the EditorTransition for a single terminal.
//
// SBNF pop=0 / pop=1 mapping to EditorTransition:
//   - pop=0 + no contexts  → STACK_NONE
//   - pop=0 + contexts     → STACK_PUSH  (loop context stays on Sublime stack)
//   - pop=1 + no contexts  → STACK_POP(1)
//   - pop=1 + contexts     → STACK_SET
//
// GNest terminals (term.nestNode != nil) always emit STACK_PUSH to an independent
// body context so that recursive expression grammars don't cause infinite expansion.
func (bs *lsBuildState[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTransition(
term lsTerminal[TToken, TNodeKind],
_ string, // currentKey – reserved for future simple-repetition optimisation
) *EditorTransition[TObservation, TContext] {

cfg := bs.gen.config
tokenRule, hasTokenRule := bs.gen.tokenToRule[term.token]

edCtx := &EditorCtx[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{Token: &term.token}
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
MatchContext:  matchCtx,
Captures:      captures,
Operation:     STACK_PUSH,
Targets:       []*EditorState[TObservation, TContext]{bodyState},
}
}

// GNest push-boundary: open token → push a self-contained body context.
// The body context is computed without the outer continuation stack, which
// terminates recursion for grammars that allow nested expressions.
if term.nestNode != nil {
bodyState := bs.getOrCreateNestBodyContext(term.nestNode)
ownerLabel2 := lsOwningRule(term)
// Compute the outer continuation (what follows this nest in the parent grammar).
// nestNode is cleared so advanceTerminal treats remaining/stack normally.
noNest := lsTerminal[TToken, TNodeKind]{token: term.token, remaining: term.remaining, stack: term.stack}
advTerminals := lsAdvanceTerminal(noNest, bs.rules)
if advTerminals == nil {
return &EditorTransition[TObservation, TContext]{
OnPattern:   pat,
MatchContext: matchCtx,
Captures:     captures,
Operation:    STACK_PUSH,
Targets:      []*EditorState[TObservation, TContext]{bodyState},
}
}
afterNest := bs.getOrCreateContext(advTerminals, ownerLabel2)
return &EditorTransition[TObservation, TContext]{
OnPattern:   pat,
MatchContext: matchCtx,
Captures:     captures,
Operation:    STACK_PUSH,
// afterNest pushed first (goes deeper), bodyState on top (processed first).
Targets:      []*EditorState[TObservation, TContext]{afterNest, bodyState},
}
}

// SBNF repetition / continuation logic.
ownerLabel := lsOwningRule(term)
isRepetition := len(term.stack) > 0 && term.stack[len(term.stack)-1].isRepetition

var advTerminals []lsTerminal[TToken, TNodeKind]

if isRepetition {
// Advance without the outermost Repetition frame (iteration continuation only).
// The loop context remains on the Sublime stack via PUSH (pop=0).
stripped := lsTerminal[TToken, TNodeKind]{
token:     term.token,
remaining: term.remaining,
stack:     term.stack[:len(term.stack)-1],
}
advTerminals = lsAdvanceTerminal(stripped, bs.rules)
} else {
advTerminals = lsAdvanceTerminal(term, bs.rules)
}

if isRepetition {
// pop=0: keep the loop context (current state) on the Sublime stack.
if advTerminals == nil {
return &EditorTransition[TObservation, TContext]{
OnPattern:    pat,
MatchContext:  matchCtx,
Captures:      captures,
Operation:     STACK_NONE,
}
}
next := bs.getOrCreateContext(advTerminals, ownerLabel)
return &EditorTransition[TObservation, TContext]{
OnPattern:    pat,
MatchContext:  matchCtx,
Captures:      captures,
Operation:     STACK_PUSH,
Targets:       []*EditorState[TObservation, TContext]{next},
}
}

// pop=1: replace current context with continuation, or just pop.
if advTerminals == nil {
return &EditorTransition[TObservation, TContext]{
OnPattern:    pat,
MatchContext:  matchCtx,
Captures:      captures,
Operation:     STACK_POP,
PopAmount:     1,
}
}
next := bs.getOrCreateContext(advTerminals, ownerLabel)
return &EditorTransition[TObservation, TContext]{
OnPattern:    pat,
MatchContext:  matchCtx,
Captures:      captures,
Operation:     STACK_SET,
Targets:       []*EditorState[TObservation, TContext]{next},
}
}

// getOrCreateNestBodyContext returns the EditorState for the interior of a GNest.
// The context is keyed by the nest node's GrammarLabel so that the same nest
// reused at different call sites always maps to the same Sublime context
// (allowing correct push/pop for balanced delimiters at any nesting depth).
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

s := &EditorState[TObservation, TContext]{ID: name, Label: name}
// Register BEFORE computing body so recursive nests find this state and reuse it.
bs.contextByKey[nestKey] = s
bs.states = append(bs.states, s)

// Compute body lookahead FRESH (no outer continuation), appending the close
// token so the natural end of the body chain emits a POP for the close token.
// This prevents recursive expression grammars from expanding infinitely while
// ensuring the close token is correctly handled at the end of the sequence.
closeTokenNode := &syntaxa.Grammar[TToken, TNodeKind]{Kind: syntaxa.GToken, Token: *nestNode.CloseToken}
bodyAndClose := []*syntaxa.Grammar[TToken, TNodeKind]{nestNode.Children[0], closeTokenNode}
bodyTerminals, _ := lsLookaheadConcat[TToken, TNodeKind](bodyAndClose, bs.rules, make(lsVisiting))

if len(bodyTerminals) > 0 {
bs.queue = append(bs.queue, lsPendingCtx[TObservation, TToken, TNodeKind, TContext]{
state:     s,
ctxKey:    nestKey,
terminals: bodyTerminals,
})
}

return s
}

// ------------------------------------------------------------- HELPERS

func (e *editorIRGenerator[TObservation, TToken, TTokenRole, TLexerState, TNodeKind, TContext]) buildTokenMap() {
for _, rule := range e.lexingRuleset.GetRules() {
e.tokenToRule[rule.Token] = rule
}
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
MatchContext:  dp.CloseContext,
Operation:     STACK_POP,
PopAmount:     1,
},
},
}
e.delimitedStates[dp.StateLabel] = s
return s
}
