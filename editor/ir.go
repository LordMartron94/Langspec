package editor

import (
	"autarch/pattern"
	"cmp"
	"fmt"
	"foundation/bytes"
	"foundation/formatting"
	"foundation/hash"
	"lexarch"
	"slices"
	"strings"
	"syntaxa"
)

var xxh3Hasher = hash.XXH3HasherCreateWithSeed(42)

// ------------------------------------------------------------------ TYPES & CONSTANTS

/*
Pattern is the editor's pattern type: a Regula AST over runes used for regex emission.
*/
type Pattern = pattern.RegulaAST[rune]

/*
StateID uniquely identifies a state in the push-down automaton. Opaque; used for includes and action targets.
*/
type StateID uint64

/*
StateRuleID uniquely identifies a rule within the IR. Opaque; used for fallback and capture rules.
*/
type StateRuleID uint64

//go:generate stringer -type RuleAction
/*
RuleAction is the action performed when a rule matches: push (enter context), pop (exit), set (replace top), match (consume), embed, or none.
*/
type RuleAction uint8

const (
	ACTION_PUSH  RuleAction = iota // Enter a new context; ActionTarget is the state to push.
	ACTION_POP                     // Exit one or more contexts.
	ACTION_SET                     // Replace top of stack; ActionTarget is the new state.
	ACTION_MATCH                   // Consume input and stay in current state.
	ACTION_NONE                    // No action.
	ACTION_EMBED                   // Embed another syntax until escape; uses Embed, EmbedScope, Escape, EscapeCaptures.
)

const (
	invalidFallbackLabel     = "invalid_fallback"
	invalidFallbackBaseScope = "invalid.illegal.unexpected-token"
)

// Rule priority for ordering: lower value = earlier in output (higher precedence).
// Generators (e.g. Sublime) emit rules with Priority < PriorityAfterIncludes first, then includes, then rules with Priority >= PriorityAfterIncludes.
const (
	PriorityDefault       = 0    // Normal rules (emit before includes)
	PriorityAfterIncludes = 500  // Threshold: rules with priority >= this are emitted after the includes block
	PriorityFallback      = 1000 // Fallback / invalid-token rules (emit last, after includes)
)

/*
State is one context in the push-down automaton. It has a unique ID, label (used in includes and YAML output),
optional meta scope, rules (match patterns and actions), and includes (whitelist of other state IDs that may be entered).
IsRootContext marks states that are included from the root "main" context. OmitPrototype excludes the state from prototype includes.
*/
type State struct {
	ID            StateID
	Label         string
	MetaScope     string
	Rules         []StateRule
	Includes      []StateID
	IsRootContext bool
	OmitPrototype bool
}

/*
StateRule is one match rule within a state. RegEx is the pattern to match; Scope is applied on match.
Action and ActionTarget define the transition (push/pop/set/match). Captures, Embed, EmbedScope, Escape, EscapeCaptures
support capture groups and embedded syntax. PopCount is used when Action is ACTION_POP to pop multiple levels.
Priority orders the rule relative to others in the same state: lower value = earlier (higher precedence). Use PriorityDefault and PriorityFallback.
*/
type StateRule struct {
	ID             StateRuleID
	Label          string
	RegEx          string
	Scope          string
	Action         RuleAction
	ActionTarget   StateID
	Captures       map[int]string
	PopCount       int
	Embed          string
	EmbedScope     string
	Escape         string
	EscapeCaptures map[int]string
	Priority       int
}

/*
ScopeProvider returns the scope string for a token. Used when building default rules and overrides.
*/
type ScopeProvider[TToken any] func(item TToken) string

/*
TokenFormatter returns a display string for a token. Used for state labels and trigger IDs.
*/
type TokenFormatter[TToken any] func(token TToken) string

/*
MustGetRegEx returns the regex string for the token from the lexing ruleset. Panics if the token is not found or ToRegEx fails.
Use when building overrides or rules that require a pattern for a known token.
*/
func MustGetRegEx[TToken, TTokenRole comparable](
	token TToken,
	ruleset *lexarch.LexingRuleset[rune, TToken, TTokenRole],
) string {
	pattern, ok := ruleset.GetPattern(token)
	if !ok {
		panic(fmt.Sprintf("editor.MustGetRegEx: token %v not found in lexing ruleset", token))
	}

	regex, err := pattern.ToRegEx()
	if err != nil {
		panic(fmt.Sprintf("editor.MustGetRegEx: failed to emit regex for token %v: %v", token, err))
	}
	return regex
}

// ------------------------------------------------------------------ OVERRIDE CONTEXTS

/*
TokenOverrideContext is passed to TokenOverrideFunc. It provides BaseID, Label, OriginalPattern, BaseScope, and ScopeExtension.
Use DeriveStateID(suffix) and ApplyScope(scope) when building override rules and extra states.
*/
type TokenOverrideContext struct {
	BaseID          StateID
	Label           string
	OriginalPattern Pattern
	BaseScope       string
	ScopeExtension  string
}

func (ctx *TokenOverrideContext) DeriveStateID(suffix string) StateID {
	return StateID(produceStateID(fmt.Sprintf("%s_%s", ctx.Label, suffix)))
}

func (ctx *TokenOverrideContext) ApplyScope(scopes ...string) string {
	return getScopeString(scopes, ctx.ScopeExtension)
}

/*
TokenOverrideFunc builds the main StateRule and optional extra States for a token override. Called by the IR when the token has an override registered.
*/
type TokenOverrideFunc func(ctx *TokenOverrideContext) (mainRule StateRule, extraStates []State)

/*
NestOverrideContext is passed to NestOverrideFunc. It provides NestLabel, ScopeExtension, and GetRegEx(token). Use DeriveStateID and ApplyScope when building nest states.
*/
type NestOverrideContext[TToken comparable] struct {
	NestLabel      string
	ScopeExtension string
	getRegEx       func(TToken) string
}

func (ctx *NestOverrideContext[TToken]) GetRegEx(token TToken) string {
	return ctx.getRegEx(token)
}

func (ctx *NestOverrideContext[TToken]) DeriveStateID(suffix string) StateID {
	return StateID(produceStateID(fmt.Sprintf("%s_%s", ctx.NestLabel, suffix)))
}

func (ctx *NestOverrideContext[TToken]) ApplyScope(scopes ...string) string {
	return getScopeString(scopes, ctx.ScopeExtension)
}

/*
NestOverrideFunc returns the entry state ID and the slice of states for a custom nest. Called when a nest matches the registered NestOverridePredicate.
*/
type NestOverrideFunc[TToken comparable] func(ctx *NestOverrideContext[TToken]) (entryStateID StateID, states []State)

/*
NestOverridePredicate returns true if the nest should use the associated NestOverrideFunc. Used with AddNestOverrideByPredicate.
*/
type NestOverridePredicate[TToken comparable] func(nest *syntaxa.NestSpec[TToken]) bool

// ------------------------------------------------------------------ CONFIGURATION

/*
OverrideConfig holds optional scope and meta-scope for a grammar node override. HasScope / HasMetaScope / HasAny report which fields are set.
*/
type OverrideConfig[TToken comparable] struct {
	Scopes      []string
	TokenScopes map[TToken][]string
	MetaScope   string
}

func (c OverrideConfig[TToken]) HasAnyScope() bool {
	return len(c.Scopes) > 0 || len(c.TokenScopes) > 0
}

func (c OverrideConfig[TToken]) HasMetaScope() bool {
	return c.MetaScope != ""
}

type nestOverrideHandler[TToken comparable] struct {
	pred NestOverridePredicate[TToken]
	fn   NestOverrideFunc[TToken]
}

/*
PushDownAutomatonIRConfiguration holds scope provider, token formatter, scope extension, token overrides, nest override handlers, prototype token roles, and node overrides.
Create with PushDownAutomatonIRConfigurationCreate; then use AddOverride, AddNestOverride, AddNodeOverride, AddPrototypeTokenRoles as needed.
*/
type PushDownAutomatonIRConfiguration[TToken, TTokenRole comparable] struct {
	scopeProvider        ScopeProvider[TToken]
	formatter            TokenFormatter[TToken]
	scopeExtension       string
	overrides            map[TToken]TokenOverrideFunc
	prototypeTokenRoles  []TTokenRole
	nestOverrideHandlers []nestOverrideHandler[TToken]
	nodeOverrides        map[syntaxa.GrammarLabel]OverrideConfig[TToken]
}

/*
PushDownAutomatonIRConfigurationCreate allocates a new configuration with the given scope provider, formatter, and scope extension. Add overrides and prototype roles before calling PushDownAutomatonIRCreate.
*/
func PushDownAutomatonIRConfigurationCreate[TToken, TTokenRole comparable](
	provider ScopeProvider[TToken],
	formatter TokenFormatter[TToken],
	scopeExtension string,
) *PushDownAutomatonIRConfiguration[TToken, TTokenRole] {
	return &PushDownAutomatonIRConfiguration[TToken, TTokenRole]{
		scopeProvider:        provider,
		formatter:            formatter,
		scopeExtension:       scopeExtension,
		overrides:            make(map[TToken]TokenOverrideFunc),
		prototypeTokenRoles:  make([]TTokenRole, 0),
		nestOverrideHandlers: make([]nestOverrideHandler[TToken], 0),
		nodeOverrides:        make(map[syntaxa.GrammarLabel]OverrideConfig[TToken]),
	}
}

/*
AddOverride registers a token override. When the IR encounters this token, it calls fn to obtain the main rule and optional extra states instead of the default single-rule state.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddOverride(token TToken, fn TokenOverrideFunc) {
	c.overrides[token] = fn
}

/*
AddNestOverride registers a nest override for the given rule ID. The nest is identified by nest.OwnerRule == ruleID.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverride(ruleID syntaxa.GrammarLabel, fn NestOverrideFunc[TToken]) {
	c.AddNestOverrideByPredicate(func(nest *syntaxa.NestSpec[TToken]) bool {
		return nest.OwnerRule == ruleID
	}, fn)
}

/*
AddNestOverrideByPredicate registers a nest override for nests matching pred. Use when identification is not by OwnerRule alone.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverrideByPredicate(pred NestOverridePredicate[TToken], fn NestOverrideFunc[TToken]) {
	c.nestOverrideHandlers = append(c.nestOverrideHandlers, nestOverrideHandler[TToken]{pred: pred, fn: fn})
}

/*
AddNodeOverride attaches scope and/or meta-scope to a grammar node by GrammarLabel. Node must be a GToken (scope) or GConcat (meta-scope) for the override to apply.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeOverride(nodeID syntaxa.GrammarLabel, config OverrideConfig[TToken]) {
	c.nodeOverrides[nodeID] = config
}

/*
AddNodeScopeOverride is a convenience for AddNodeOverride with only Scope set. Use for token nodes that need a custom scope.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeScopeOverride(nodeID syntaxa.GrammarLabel, scopes ...string) {
	c.AddNodeOverride(nodeID, OverrideConfig[TToken]{Scopes: scopes})
}

/*
AddPrototypeTokenRoles marks tokens with the given roles as prototype contexts: they are included in the root prototype state so they can be entered from the main context.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddPrototypeTokenRoles(tokenRoles ...TTokenRole) {
	c.prototypeTokenRoles = append(c.prototypeTokenRoles, tokenRoles...)
}

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddConditionalNodeScopeOverride(nodeID syntaxa.GrammarLabel, token TToken, scopes ...string) {
	oc := c.nodeOverrides[nodeID]
	if oc.TokenScopes == nil {
		oc.TokenScopes = make(map[TToken][]string)
	}
	oc.TokenScopes[token] = scopes
	c.nodeOverrides[nodeID] = oc
}

// ------------------------------------------------------------------ GRAMMAR NODE IR ROLE

/*
EditorIRRole classifies a grammar node for IR generation: Token (terminal, emits match rule),
Nest (bracketed region), Segment (composite, includes/lookahead only), or Epsilon (empty).
Used so IR logic dispatches on role instead of GrammarKind; new kinds plug in via editorIRRole.
*/
type EditorIRRole uint8

const (
	EditorIRRoleToken   EditorIRRole = iota // Terminal; emit token match rule.
	EditorIRRoleNest                        // Bracketed open/body/close; nest states.
	EditorIRRoleSegment                     // Composite; no rule for node, only includes/lookahead.
	EditorIRRoleEpsilon                     // Empty production; skip in IR.
)

/*
editorIRRole returns the IR role for a grammar node. Handles every known GrammarKind explicitly;
unknown kinds (e.g. a new kind added in syntaxa before editor is updated) return EditorIRRoleSegment
so analysis-driven includes and lookahead still apply.
*/
func editorIRRole[TToken comparable](node *syntaxa.Grammar[TToken]) EditorIRRole {
	if node == nil {
		return EditorIRRoleEpsilon
	}
	switch node.Kind {
	case syntaxa.GToken:
		return EditorIRRoleToken
	case syntaxa.GNest:
		return EditorIRRoleNest
	case syntaxa.GConcat, syntaxa.GChoice, syntaxa.GRepeat, syntaxa.GOptional, syntaxa.GReference:
		return EditorIRRoleSegment
	case syntaxa.GEpsilon:
		return EditorIRRoleEpsilon
	default:
		// Unknown GrammarKind: treat as segment so First(node) is still used for includes.
		return EditorIRRoleSegment
	}
}

// ------------------------------------------------------------------ IR ORCHESTRATOR

/*
validateNodeOverrideLabels ensures every node override label exists in the grammar. If any override
is registered for a label that never appears in nodesByGrammarLabel, the build fails with a clear
error so that "override for a node that is never added" cannot slip through.
*/
func validateNodeOverrideLabels[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nodesByGrammarLabel map[syntaxa.GrammarLabel][]*syntaxa.Grammar[TToken],
) {
	var missing []string
	for label := range config.nodeOverrides {
		if len(nodesByGrammarLabel[label]) == 0 {
			missing = append(missing, string(label))
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		panic(fmt.Sprintf(
			"editor IR: node scope/metascope override has no matching grammar node for label(s) %s; ensure the grammar uses the same label (e.g. LangSpecGrammarIDFromNode for that node kind) so the node is actually added",
			formatting.FormatStringSlice(missing, formatting.FormatSliceOptions[string]{Separator: ", ", Quote: true}),
		))
	}
}

/*
PushDownAutomatonIR is the result of IR construction. It holds the language name and version (from the grammar package),
the slice of States (contexts and rules), and the scope extension applied to all scopes. Pass to a generator (e.g. Sublime) to emit syntax files.
*/
type PushDownAutomatonIR struct {
	LanguageName    string
	LanguageVersion string
	States          []State
	ScopeExtension  string
}

/*
PushDownAutomatonIRCreate builds the push-down automaton IR from the given configuration, lexing ruleset, and grammar package.
It builds base states from tokens and delimited rules, injects node overrides and construct states from the grammar, injects nest states and sequence triggers, adds the prototype and main states, and returns the complete IR. grammarPackage.Analysis and grammarPackage.NodesByGrammarLabel must be populated (use syntaxa.ProducePackage).
*/
func PushDownAutomatonIRCreate[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind, TLexerState comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
) *PushDownAutomatonIR {
	validateNodeOverrideLabels(config, grammarPackage.NodesByGrammarLabel)

	entryGrammar := grammarPackage.Grammars[grammarPackage.EntryRule]

	plan := BuildIRPlan(config, grammarPackage.Analysis, entryGrammar)
	tokensInUse, tokenPatternMap, prototypeTokens, tokenPriorityMap := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)
	delimitedMap := extractDelimitedRules(lexingRuleSet)

	rootTokens, _, _ := extractIncludes(config, plan, grammarPackage.Analysis, entryGrammar, true)
	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, tokenPriorityMap, prototypeTokens, rootTokens, delimitedMap)

	var nestBodyRegistry map[syntaxa.GrammarLabel]StateID
	allStates, nestRegistry, nestBodyRegistry := injectNestStatesPlanned(config, plan, allStates, grammarPackage, tokenPatternMap, tokensInUse)

	allStates = injectPlannedNodeStates(config, plan, grammarPackage.Analysis, grammarPackage.NodesByGrammarLabel, allStates, entryGrammar, tokenPatternMap, tokenPriorityMap, nestBodyRegistry, tokensInUse)
	allStates = injectPlannedSequenceTriggers(config, plan, allStates, nestRegistry, tokenPatternMap)
	allStates = injectPrototypeState(allStates, prototypeIncludes)
	allStates = injectMainStatePlanned(allStates, config.scopeExtension, config, plan, grammarPackage.Analysis, entryGrammar)

	optimizedStates := optimizeAutomaton(allStates)

	return &PushDownAutomatonIR{
		LanguageName:    grammarPackage.Name,
		LanguageVersion: grammarPackage.Version,
		States:          optimizedStates,
		ScopeExtension:  config.scopeExtension,
	}
}

// ------------------------------------------------------------------ COMPILER STAGES

func extractLexerTokens[TToken, TTokenRole comparable](
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	prototypeTokenRoles []TTokenRole,
) ([]TToken, map[TToken]Pattern, map[TToken]bool, map[TToken]int) {
	var tokensInUse []TToken
	tokenPatternMap := make(map[TToken]Pattern)
	tokenPriorityMap := make(map[TToken]int)
	prototypeTokens := make(map[TToken]bool)

	type tokenWithIndex struct {
		token TToken
		idx   int
	}
	var withIndex []tokenWithIndex
	for i, lexerRule := range lexingRuleSet.GetRules() {
		tokenPatternMap[lexerRule.Token] = lexerRule.Pattern
		tokenPriorityMap[lexerRule.Token] = lexerRule.Priority
		if slices.Contains(prototypeTokenRoles, lexerRule.Role) {
			prototypeTokens[lexerRule.Token] = true
		}
		withIndex = append(withIndex, tokenWithIndex{token: lexerRule.Token, idx: i})
	}
	slices.SortFunc(withIndex, func(a, b tokenWithIndex) int {
		pa, pb := tokenPriorityMap[a.token], tokenPriorityMap[b.token]
		if pa != pb {
			return pb - pa // higher priority first
		}
		return a.idx - b.idx // stable: preserve original order
	})
	tokensInUse = make([]TToken, 0, len(withIndex))
	for _, w := range withIndex {
		tokensInUse = append(tokensInUse, w.token)
	}
	return tokensInUse, tokenPatternMap, prototypeTokens, tokenPriorityMap
}

/*
DelimitedRuleRegex holds the open and close regex strings for a token that has a delimited rule in the lexing ruleset.
*/
type DelimitedRuleRegex[TToken comparable] struct {
	OpenRegex  string
	CloseRegex string
}

func extractDelimitedRules[TToken, TTokenRole comparable](
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
) map[TToken]DelimitedRuleRegex[TToken] {
	out := make(map[TToken]DelimitedRuleRegex[TToken])
	for _, lexerRule := range lexingRuleSet.GetRules() {
		open, close, ok := lexingRuleSet.GetDelimitedRule(lexerRule.Token)
		if !ok {
			continue
		}
		openRegex, errOpen := open.ToRegEx()
		closeRegex, errClose := close.ToRegEx()
		if errOpen == nil && errClose == nil {
			out[lexerRule.Token] = DelimitedRuleRegex[TToken]{OpenRegex: openRegex, CloseRegex: closeRegex}
		}
	}
	return out
}

func buildBaseStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	tokensInUse []TToken,
	tokenPatternMap map[TToken]Pattern,
	tokenPriorityMap map[TToken]int,
	prototypeTokens map[TToken]bool,
	rootTokens map[TToken]struct{},
	delimitedMap map[TToken]DelimitedRuleRegex[TToken],
) ([]State, []StateID) {
	var allStates []State
	var prototypeIncludes []StateID

	for _, token := range tokensInUse {
		state, extraStates, isProto := buildSingleBaseState(config, token, tokenPatternMap, tokenPriorityMap, prototypeTokens, rootTokens, delimitedMap)
		allStates = append(allStates, state)
		allStates = append(allStates, extraStates...)

		if isProto {
			prototypeIncludes = append(prototypeIncludes, state.ID)
		}
	}
	return allStates, prototypeIncludes
}

func buildSingleBaseState[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	token TToken,
	tokenPatternMap map[TToken]Pattern,
	tokenPriorityMap map[TToken]int,
	prototypeTokens map[TToken]bool,
	rootTokens map[TToken]struct{},
	delimitedMap map[TToken]DelimitedRuleRegex[TToken],
) (State, []State, bool) {
	label := sanitizeContextName(config.formatter(token))
	id := StateID(produceStateID(label))
	baseScope := config.scopeProvider(token)
	lexerPriority := tokenPriorityMap[token] // 0 if missing
	rulePriority := PriorityDefault - lexerPriority

	var mainRule StateRule
	var extraStates []State

	if delimited, hasDelimited := delimitedMap[token]; hasDelimited {
		ctx := buildTokenOverrideContext(id, label, tokenPatternMap[token], baseScope, config.scopeExtension)
		mainRule, extraStates = TokenOverrideDelimitedRegion(ctx, delimited.OpenRegex, delimited.CloseRegex, "punctuation.definition.comment.begin", baseScope, "punctuation.definition.comment.end")
		mainRule.Priority = rulePriority
	} else if overrideFn, exists := config.overrides[token]; exists {
		ctx := buildTokenOverrideContext(id, label, tokenPatternMap[token], baseScope, config.scopeExtension)
		mainRule, extraStates = overrideFn(ctx)
		mainRule.Priority = rulePriority
	} else {
		defaultRegex, _ := tokenPatternMap[token].ToRegEx()
		mainRule = StateRule{
			ID:       StateRuleID(id),
			Label:    label,
			Action:   ACTION_MATCH,
			Scope:    getScopeString([]string{baseScope}, config.scopeExtension),
			RegEx:    defaultRegex,
			Priority: rulePriority,
		}
	}

	isProto := prototypeTokens[token]
	_, isStructurallyRoot := rootTokens[token]

	baseState := State{
		ID:            id,
		Label:         label,
		Rules:         []StateRule{mainRule},
		IsRootContext: isStructurallyRoot && !isProto,
	}

	return baseState, extraStates, isProto
}

func buildTokenOverrideContext(id StateID, label string, originalPattern Pattern, baseScope, scopeExtension string) *TokenOverrideContext {
	return &TokenOverrideContext{
		BaseID:          id,
		Label:           label,
		OriginalPattern: originalPattern,
		BaseScope:       baseScope,
		ScopeExtension:  scopeExtension,
	}
}

// ------------------------------------------------------------------ NEST COMPILER

func tryApplyNestOverride[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
) ([]State, StateID, bool) {
	for _, h := range config.nestOverrideHandlers {
		if h.pred(&nest) {
			ctx := &NestOverrideContext[TToken]{
				NestLabel:      nestLabel,
				ScopeExtension: config.scopeExtension,
				getRegEx: func(tok TToken) string {
					r, _ := tokenPatternMap[tok].ToRegEx()
					return r
				},
			}
			entryStateID, customStates := h.fn(ctx)
			return customStates, entryStateID, true
		}
	}
	return nil, 0, false
}

func buildExpectState[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	expectStateID StateID,
	bodyStateID StateID,
	expectAnalysis includeAnalysis[TToken],
	tokensInUse []TToken,
) State {
	openRegex, _ := tokenPatternMap[nest.Open].ToRegEx()

	var includes []StateID

	// Inject the smart structural overrides
	includes = append(includes, expectAnalysis.TriggerIDs...)
	includes = append(includes, expectAnalysis.ConstructIDs...)
	includes = append(includes, expectAnalysis.OverrideIDs...)

	// Inject the remaining base tokens
	if len(expectAnalysis.ValidTokens) > 0 && len(tokensInUse) > 0 {
		includes = append(includes, buildIncludesWhitelist(expectAnalysis.ValidTokens, nest.Open, tokensInUse, func(tok TToken) StateID {
			return tokenToStateID(config, tok)
		})...)
	}

	return State{
		ID:            expectStateID,
		Label:         nestLabel + "_expect",
		IsRootContext: false,
		Includes:      dedupeStateIDs(includes),
		Rules: []StateRule{
			{
				ID:           StateRuleID(produceStateID(nestLabel + "_open")),
				RegEx:        openRegex,
				Scope:        getScopeString([]string{config.scopeProvider(nest.Open)}, config.scopeExtension),
				Action:       ACTION_SET,
				ActionTarget: bodyStateID,
			},
			InvalidFallbackRule(StateRuleID(produceStateID(nestLabel+"_expect_invalid")), config.scopeExtension),
		},
	}
}

func collectExpectStateIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	root *syntaxa.Grammar[TToken],
	nestID syntaxa.GrammarLabel,
) includeAnalysis[TToken] {
	out := includeAnalysis[TToken]{
		ValidTokens: make(map[TToken]struct{}),
	}
	if root == nil {
		return out
	}

	_ = root.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		if n.Kind != syntaxa.GConcat || len(n.Children) < 2 {
			return false, false
		}
		for i := 0; i < len(n.Children)-1; i++ {
			prev := n.Children[i]
			next := n.Children[i+1]

			if prev == nil || next == nil || next.Kind != syntaxa.GNest || next.GrammarLabel != nestID {
				continue
			}
			if prev.Kind == syntaxa.GToken {
				continue
			}

			// For everything between the trigger token and the nest open:
			for j := 1; j < len(prev.Children); j++ {
				sub := traverseIncludes(config, plan, analysis, prev.Children[j], false)

				for t := range sub.ValidTokens {
					out.ValidTokens[t] = struct{}{}
				}
				out.ConstructIDs = append(out.ConstructIDs, sub.ConstructIDs...)
				out.OverrideIDs = append(out.OverrideIDs, sub.OverrideIDs...)
				out.TriggerIDs = append(out.TriggerIDs, sub.TriggerIDs...)
			}
		}
		return false, false
	})

	return out
}

func mutateStateAction(allStates []State, targetStateID StateID, action RuleAction, actionTarget StateID) {
	for i := range allStates {
		if allStates[i].ID == targetStateID && len(allStates[i].Rules) > 0 {
			allStates[i].Rules[0].Action = action
			allStates[i].Rules[0].ActionTarget = actionTarget
			return
		}
	}
}

// ------------------------------------------------------------------ NODE OVERRIDES & CONSTRUCTS
//
// Construct state extraction walks the grammar via GrammarWalkPreWithContext, collects
// GConcat nodes planned as constructs, and builds state chains from constructBuildEnv
// and constructChainCtx. One env is shared for the run; one chain context per GConcat.

// constructBuildEnv holds shared context for the construct-state pipeline (config, plan, analysis, token patterns, token priorities).
type constructBuildEnv[TToken, TTokenRole comparable] struct {
	Config           *PushDownAutomatonIRConfiguration[TToken, TTokenRole]
	Plan             *IRPlan[TToken]
	Analysis         *syntaxa.GrammarAnalysis[TToken]
	TokenPatternMap  map[TToken]Pattern
	TokenPriorityMap map[TToken]int
	NestBodyRegistry map[syntaxa.GrammarLabel]StateID
	TokensInUse      []TToken
}

// constructChainCtx holds context for building one construct state chain (one GConcat). Derived fields are set by buildConstructStateChain.
type constructChainCtx[TToken, TTokenRole comparable] struct {
	Env                 *constructBuildEnv[TToken, TTokenRole]
	ConcatNode          *syntaxa.Grammar[TToken]
	MetaScope           string
	InheritedSyncTokens []TToken
	IsRepeating         bool
	BaseLabel           string
	StepCount           int
	RecoveryStateID     StateID
}

func nodeOverrideStateID(grammarID syntaxa.GrammarLabel) StateID {
	return StateID(produceStateID(fmt.Sprintf("node_override_%s", sanitizeContextName(string(grammarID)))))
}

func nodeConstructEntryStateID(concatID syntaxa.GrammarLabel) StateID {
	return StateID(produceStateID(fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatID)))))
}

func buildConstructStateChain[TToken, TTokenRole comparable](
	env *constructBuildEnv[TToken, TTokenRole],
	concatNode *syntaxa.Grammar[TToken],
	metaScope string,
	inheritedSyncTokens []TToken,
	isRepeating bool,
) []State {
	baseLabel := fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatNode.GrammarLabel)))
	stepCount := len(concatNode.Children)
	recoveryStateID := StateID(produceStateID(baseLabel + "_recovery"))

	currentSyncTokens := dedupeSyncTokens(append(append([]TToken(nil), inheritedSyncTokens...), concatNode.RecoveryTokens...))

	chainCtx := &constructChainCtx[TToken, TTokenRole]{
		Env:                 env,
		ConcatNode:          concatNode,
		MetaScope:           metaScope,
		InheritedSyncTokens: inheritedSyncTokens,
		IsRepeating:         isRepeating,
		BaseLabel:           baseLabel,
		StepCount:           stepCount,
		RecoveryStateID:     recoveryStateID,
	}

	var states []State
	for i := 0; i < stepCount; i++ {
		states = append(states, buildConstructStep(chainCtx, i))
	}
	states = append(states, buildRecoveryState(recoveryStateID, baseLabel+"_recovery", currentSyncTokens, env.TokenPatternMap, env.Config.scopeExtension))
	return states
}

// dedupeSlice returns a slice with duplicate elements removed; first occurrence kept, order preserved.
func dedupeSlice[T comparable](s []T) []T {
	if len(s) <= 1 {
		return s
	}
	seen := make(map[T]struct{}, len(s))
	out := make([]T, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// dedupeSyncTokens returns a slice with duplicate tokens removed (first occurrence kept).
func dedupeSyncTokens[TToken comparable](tokens []TToken) []TToken {
	return dedupeSlice(tokens)
}

func buildRecoveryState[TToken comparable](
	id StateID,
	label string,
	syncTokens []TToken,
	tokenPatternMap map[TToken]Pattern,
	scopeExtension string,
) State {
	var rules []StateRule
	seenRegex := make(map[string]struct{})

	// 1. Peek for synchronization tokens to escape the panic state (one rule per unique pattern)
	for _, tok := range syncTokens {
		regex, err := tokenPatternMap[tok].ToRegEx()
		if err != nil || regex == "" {
			continue
		}
		pattern := fmt.Sprintf(`(?=\s*(?:%s))`, regex)
		if _, ok := seenRegex[pattern]; ok {
			continue
		}
		seenRegex[pattern] = struct{}{}
		rules = append(rules, StateRule{
			ID:       StateRuleID(produceStateID(fmt.Sprintf("%s_sync_%v", label, tok))),
			Label:    fmt.Sprintf("sync_%v", tok),
			RegEx:    pattern,
			Action:   ACTION_POP,
			Priority: PriorityDefault,
		})
	}

	// 2. Consume garbage if no sync token is in sight
	rules = append(rules, InvalidFallbackRule(StateRuleID(produceStateID(label+"_invalid")), scopeExtension))

	return State{
		ID:            id,
		Label:         label,
		Rules:         rules,
		IsRootContext: false,
	}
}

func buildConstructStep[TToken, TTokenRole comparable](chainCtx *constructChainCtx[TToken, TTokenRole], index int) State {
	child := chainCtx.ConcatNode.Children[index]
	baseLabel := chainCtx.BaseLabel
	stepCount := chainCtx.StepCount
	lbl := stepLabel(baseLabel, index, stepCount)
	id := stepID(baseLabel, chainCtx.ConcatNode.GrammarLabel, index, stepCount)
	role := editorIRRole(child)

	config := chainCtx.Env.Config
	var rules []StateRule
	var includes []StateID

	switch role {
	case EditorIRRoleNest:
		rules, _ = handleNestConstructStep(chainCtx, child, index, lbl, nil)
		includes = nil
	case EditorIRRoleToken:
		includes = buildIncludesForNode(config, chainCtx.Env.Analysis, child, chainCtx.Env.TokensInUse)
		rules, includes = handleTokenConstructStep(chainCtx, child, index, lbl, includes)
	case EditorIRRoleSegment, EditorIRRoleEpsilon:
		includes = buildIncludesForNode(config, chainCtx.Env.Analysis, child, chainCtx.Env.TokensInUse)
		rules, includes = handleSegmentConstructStep(chainCtx, child, index, lbl, includes)
	default:
		includes = buildIncludesForNode(config, chainCtx.Env.Analysis, child, chainCtx.Env.TokensInUse)
		rules, includes = handleSegmentConstructStep(chainCtx, child, index, lbl, includes)
	}

	if index > 0 {
		rules = appendStrictSequenceBailout(rules, lbl, chainCtx.RecoveryStateID)
	}

	st := State{
		ID:       id,
		Label:    lbl,
		Rules:    rules,
		Includes: dedupeStateIDs(includes),
	}
	if index > 0 && chainCtx.MetaScope != "" {
		st.MetaScope = getScopeString([]string{chainCtx.MetaScope}, config.scopeExtension)
	}
	return st
}

func handleNestConstructStep[TToken, TTokenRole comparable](
	chainCtx *constructChainCtx[TToken, TTokenRole],
	child *syntaxa.Grammar[TToken],
	index int,
	lbl string,
	includes []StateID,
) ([]StateRule, []StateID) {
	targetID := chainCtx.Env.NestBodyRegistry[child.GrammarLabel]
	regex, _ := chainCtx.Env.TokenPatternMap[*child.OpenToken].ToRegEx()

	pushRule := StateRule{
		ID:           StateRuleID(produceStateID(lbl + "_push")),
		Label:        lbl + "_push",
		RegEx:        regex,
		Scope:        getScopeString([]string{chainCtx.Env.Config.scopeProvider(*child.OpenToken)}, chainCtx.Env.Config.scopeExtension),
		Action:       ACTION_PUSH,
		ActionTarget: targetID,
	}

	action, nextID := getTransition(baseLabelForTransition(chainCtx.ConcatNode.GrammarLabel), index, index+1, chainCtx.StepCount, chainCtx.IsRepeating, chainCtx.RecoveryStateID)

	advanceRule := StateRule{
		ID:           StateRuleID(produceStateID(lbl + "_advance")),
		Label:        lbl + "_advance",
		RegEx:        `(?=\S)`,
		Action:       action,
		ActionTarget: nextID,
		Priority:     PriorityFallback - 1,
	}

	return []StateRule{pushRule, advanceRule}, includes
}

func handleTokenConstructStep[TToken, TTokenRole comparable](
	chainCtx *constructChainCtx[TToken, TTokenRole],
	child *syntaxa.Grammar[TToken],
	index int,
	lbl string,
	includes []StateID,
) ([]StateRule, []StateID) {
	env := chainCtx.Env
	action, nextID := getTransition(baseLabelForTransition(chainCtx.ConcatNode.GrammarLabel), index, index+1, chainCtx.StepCount, chainCtx.IsRepeating, chainCtx.RecoveryStateID)

	if _, hasTokOverride := env.Config.overrides[child.Token]; hasTokOverride {
		return buildLookaheadRules(chainCtx, index, action, nextID), includes
	}
	return generateRulesForNode(env.Config, child, env.TokenPatternMap, env.TokenPriorityMap, nextID, lbl, action), nil
}

func handleSegmentConstructStep[TToken, TTokenRole comparable](
	chainCtx *constructChainCtx[TToken, TTokenRole],
	child *syntaxa.Grammar[TToken],
	index int,
	lbl string,
	includes []StateID,
) ([]StateRule, []StateID) {
	action, nextID := getTransition(baseLabelForTransition(chainCtx.ConcatNode.GrammarLabel), index, index+1, chainCtx.StepCount, chainCtx.IsRepeating, chainCtx.RecoveryStateID)
	rules := buildLookaheadRules(chainCtx, index, action, nextID)
	if child != nil && child.Kind == syntaxa.GOptional {
		for i := range rules {
			rules[i].Priority = PriorityAfterIncludes
		}
	}
	return rules, includes
}

func buildLookaheadRules[TToken, TTokenRole comparable](
	chainCtx *constructChainCtx[TToken, TTokenRole],
	index int,
	action RuleAction,
	nextID StateID,
) []StateRule {
	stepCount := chainCtx.StepCount
	if index+1 >= stepCount {
		return nil
	}
	lbl := stepLabel(chainCtx.BaseLabel, index, stepCount)
	suffixFirst := syntaxa.GrammarAnalysisFirstOfSuffix(chainCtx.Env.Analysis, chainCtx.ConcatNode, index+1)
	if la, ok := buildLookaheadForTokens(suffixFirst, chainCtx.Env.TokenPatternMap); ok {
		return []StateRule{{
			ID:           StateRuleID(produceStateID(lbl + "_advance_la")),
			Label:        lbl + "_advance_la",
			RegEx:        la,
			Action:       action,
			ActionTarget: nextID,
		}}
	}
	return nil
}

func getTransition(baseLabel string, currentIndex, targetIndex, stepCount int, isRepeating bool, recoveryStateID StateID) (RuleAction, StateID) {
	if targetIndex >= stepCount {
		if !isRepeating {
			return ACTION_SET, recoveryStateID
		}

		if currentIndex == 0 {
			return ACTION_MATCH, 0
		}
		return ACTION_POP, 0
	}

	targetID := StateID(produceStateID(fmt.Sprintf("%s_step_%d", baseLabel, targetIndex)))
	if currentIndex == 0 {
		return ACTION_PUSH, targetID
	}
	return ACTION_SET, targetID
}

func stepLabel(baseLabel string, index, stepCount int) string {
	if stepCount <= 1 {
		return baseLabel
	}
	return fmt.Sprintf("%s_step_%d", baseLabel, index)
}

func stepID(baseLabel string, grammarID syntaxa.GrammarLabel, index, stepCount int) StateID {
	if index == 0 {
		return nodeConstructEntryStateID(grammarID)
	}
	return StateID(produceStateID(stepLabel(baseLabel, index, stepCount)))
}

func baseLabelForTransition(grammarID syntaxa.GrammarLabel) string {
	return fmt.Sprintf("node_construct_%s", sanitizeContextName(string(grammarID)))
}

func buildIncludesForNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	analysis *syntaxa.GrammarAnalysis[TToken],
	node *syntaxa.Grammar[TToken],
	tokensInUse []TToken,
) []StateID {
	validTokens, overrideIDs, triggerIDs := extractIncludes(config, nil, analysis, node, false)

	if node != nil && node.Kind == syntaxa.GNest && node.OpenToken != nil {
		validTokens[*node.OpenToken] = struct{}{}
	}

	var includes []StateID
	includes = append(includes, triggerIDs...)
	includes = append(includes, overrideIDs...)
	includes = append(includes, buildIncludesWhitelist(validTokens, *new(TToken), tokensInUse, func(tok TToken) StateID {
		return tokenToStateID(config, tok)
	})...)
	return dedupeStateIDs(includes)
}

func injectPlannedNodeStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	nodesByGrammarLabel map[syntaxa.GrammarLabel][]*syntaxa.Grammar[TToken],
	allStates []State,
	grammarNode *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
	tokenPriorityMap map[TToken]int,
	nestBodyRegistry map[syntaxa.GrammarLabel]StateID,
	tokensInUse []TToken,
) []State {
	for label := range plan.OverrideTokens {
		nodes := nodesByGrammarLabel[label]
		if len(nodes) == 0 {
			continue
		}
		oc := config.nodeOverrides[label]
		if !oc.HasAnyScope() {
			continue
		}
		baseLabel := sanitizeContextName(string(label))
		stateID := nodeOverrideStateID(label)

		seenToken := make(map[TToken]struct{})
		var rules []StateRule
		for _, n := range nodes {
			if n.Kind != syntaxa.GToken {
				continue
			}
			if _, seen := seenToken[n.Token]; seen {
				continue
			}
			seenToken[n.Token] = struct{}{}

			nodeScopes := oc.Scopes
			if ts, match := oc.TokenScopes[n.Token]; match {
				nodeScopes = ts
			}

			if len(nodeScopes) == 0 {
				continue
			}

			lexerPriority := tokenPriorityMap[n.Token]
			rulePriority := PriorityDefault - lexerPriority
			regex, _ := tokenPatternMap[n.Token].ToRegEx()
			rules = append(rules, StateRule{
				ID:       StateRuleID(produceStateID(fmt.Sprintf("node_override_%s_match_%v", baseLabel, n.Token))),
				Label:    fmt.Sprintf("node_override_%s_match", baseLabel),
				Action:   ACTION_MATCH,
				Scope:    getScopeString(nodeScopes, config.scopeExtension),
				RegEx:    regex,
				Priority: rulePriority,
			})
		}
		if len(rules) == 0 {
			continue
		}
		allStates = append(allStates, State{
			ID:    stateID,
			Label: fmt.Sprintf("node_override_%s", baseLabel),
			Rules: rules,
		})
	}

	env := &constructBuildEnv[TToken, TTokenRole]{
		Config:           config,
		Plan:             plan,
		Analysis:         analysis,
		TokenPatternMap:  tokenPatternMap,
		TokenPriorityMap: tokenPriorityMap,
		NestBodyRegistry: nestBodyRegistry,
		TokensInUse:      tokensInUse,
	}
	allStates = append(allStates, extractConstructStates(env, grammarNode)...)
	return allStates
}

// constructTraverseCtx carries inherited context when walking the grammar to collect construct states (sync tokens and repeat nesting).
type constructTraverseCtx[TToken comparable] struct {
	SyncTokens []TToken
	InRepeat   bool
}

func extractConstructStates[TToken, TTokenRole comparable](
	env *constructBuildEnv[TToken, TTokenRole],
	root *syntaxa.Grammar[TToken],
) []State {
	if root == nil {
		return nil
	}

	config := env.Config
	plan := env.Plan
	var states []State
	initial := constructTraverseCtx[TToken]{SyncTokens: nil, InRepeat: false}
	_ = syntaxa.GrammarWalkPreWithContext(root, initial, func(node *syntaxa.Grammar[TToken], ctx constructTraverseCtx[TToken]) (constructTraverseCtx[TToken], bool, bool) {
		currentInRepeat := ctx.InRepeat
		switch node.Kind {
		case syntaxa.GRepeat:
			currentInRepeat = true
		case syntaxa.GNest:
			currentInRepeat = false
		case syntaxa.GToken, syntaxa.GConcat, syntaxa.GChoice, syntaxa.GOptional, syntaxa.GEpsilon:
			// No change to InRepeat; inherit from context.
		default:
			// Unknown GrammarKind: preserve context so IR generation does not assume repeat.
			currentInRepeat = ctx.InRepeat
		}

		tokenSet := make(map[TToken]struct{})
		var currentSync []TToken
		for _, tok := range append(ctx.SyncTokens, node.RecoveryTokens...) {
			if _, exists := tokenSet[tok]; !exists {
				tokenSet[tok] = struct{}{}
				currentSync = append(currentSync, tok)
			}
		}
		childCtx := constructTraverseCtx[TToken]{SyncTokens: currentSync, InRepeat: currentInRepeat}

		if node.Kind == syntaxa.GConcat {
			if _, ok := plan.ConstructConacts[node.GrammarLabel]; ok {
				var metaScope string
				if oc, ok2 := config.nodeOverrides[node.GrammarLabel]; ok2 && oc.HasMetaScope() {
					metaScope = oc.MetaScope
				}
				states = append(states, buildConstructStateChain(env, node, metaScope, currentSync, currentInRepeat)...)
			}
		}

		return childCtx, false, false
	})
	return states
}

// ------------------------------------------------------------------ SEQUENCE TRIGGER GENERATION

func injectPlannedSequenceTriggers[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	allStates []State,
	nestRegistry map[syntaxa.GrammarLabel]StateID,
	tokenPatternMap map[TToken]Pattern,
) []State {
	for _, spec := range plan.SeqTriggers {
		targetStateID, ok := nestRegistry[spec.TargetNest]
		if !ok {
			continue
		}
		triggerRegex, _ := tokenPatternMap[spec.Token].ToRegEx()

		allStates = append(allStates, State{
			ID:    spec.ID,
			Label: spec.Label,
			Rules: []StateRule{{
				ID:           StateRuleID(spec.ID),
				Label:        spec.Label + "_match",
				Action:       ACTION_PUSH,
				ActionTarget: targetStateID,
				Scope:        getScopeString([]string{config.scopeProvider(spec.Token)}, config.scopeExtension),
				RegEx:        triggerRegex,
			}},
		})
	}
	return allStates
}

func deriveTriggerID(tokenLabel string, targetID syntaxa.GrammarLabel) StateID {
	cleanToken := sanitizeContextName(tokenLabel)
	cleanTarget := sanitizeContextName(string(targetID))
	return StateID(produceStateID(fmt.Sprintf("seq_trigger_%s_to_%s", cleanToken, cleanTarget)))
}

// ------------------------------------------------------------------ CONTEXT EXTRACTION & INJECTION

type includeAnalysis[TToken comparable] struct {
	ValidTokens      map[TToken]struct{}
	SuppressedTokens map[TToken]struct{}
	TriggerIDs       []StateID
	OverrideIDs      []StateID
	ConstructIDs     []StateID
}

func extractIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken], // Optional: Pass nil to ignore plan checks
	analysis *syntaxa.GrammarAnalysis[TToken],
	contextRoot *syntaxa.Grammar[TToken],
	isRoot bool,
) (map[TToken]struct{}, []StateID, []StateID) {
	a := traverseIncludes(config, plan, analysis, contextRoot, isRoot)

	var overrides []StateID
	overrides = append(overrides, a.ConstructIDs...)
	overrides = append(overrides, a.OverrideIDs...)

	return a.ValidTokens, overrides, a.TriggerIDs
}

func traverseIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	root *syntaxa.Grammar[TToken],
	isRoot bool,
) includeAnalysis[TToken] {
	out := includeAnalysis[TToken]{
		ValidTokens:      make(map[TToken]struct{}),
		SuppressedTokens: make(map[TToken]struct{}),
	}

	if root == nil {
		return out
	}

	if root.Kind == syntaxa.GNest && root.OpenToken != nil {
		out.ValidTokens[*root.OpenToken] = struct{}{}
	}

	seenTriggers := make(map[StateID]bool)
	seenOverrides := make(map[StateID]bool)
	seenConstructs := make(map[StateID]bool)

	_ = root.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		// 1. Semantic Discovery: Add everything the grammar says is valid at this point
		if analysis != nil && n != nil && n.NodePath != nil {
			first := syntaxa.GrammarAnalysisFirst(analysis, n)
			for tok := range first {
				out.ValidTokens[tok] = struct{}{}
			}
		}

		// 2. Structural Construction: Handled by custom state chains
		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			if plan == nil || plan.containsConstruct(n.GrammarLabel) {
				cid := nodeConstructEntryStateID(n.GrammarLabel)
				if !seenConstructs[cid] {
					seenConstructs[cid] = true
					out.ConstructIDs = append(out.ConstructIDs, cid)
					// We don't suppress tokens here because constructs typically
					// encapsulate multiple steps, but watch this if you see overlap.
				}
				return true, false
			}
		}

		// 3. Sequence Triggers: Hijack the token for a specific transition
		if n.Kind == syntaxa.GConcat {
			for _, p := range syntaxa.GrammarSequenceTokenNestPairs(analysis, n) {
				id := deriveTriggerID(config.formatter(p.Token), p.NestID)
				if !seenTriggers[id] {
					seenTriggers[id] = true
					out.TriggerIDs = append(out.TriggerIDs, id)
					out.SuppressedTokens[p.Token] = struct{}{}
				}
			}
		}

		// 4. Role-based overrides and base token inclusion
		switch editorIRRole(n) {
		case EditorIRRoleToken:
			if oc, ok := config.nodeOverrides[n.GrammarLabel]; ok && oc.HasAnyScope() {
				oid := nodeOverrideStateID(n.GrammarLabel)
				if !seenOverrides[oid] {
					seenOverrides[oid] = true
					out.OverrideIDs = append(out.OverrideIDs, oid)
					out.SuppressedTokens[n.Token] = struct{}{}
				}
			} else {
				out.ValidTokens[n.Token] = struct{}{}
			}

		case EditorIRRoleNest:
			if isRoot || n != root {
				if n.OpenToken != nil {
					out.ValidTokens[*n.OpenToken] = struct{}{}
				}
				return true, false
			}
		}

		return false, false
	})

	// Final cleanup: Remove any tokens that were "promoted" to triggers or overrides
	for tok := range out.SuppressedTokens {
		delete(out.ValidTokens, tok)
	}

	return out
}

func injectPrototypeState(allStates []State, prototypeIncludes []StateID) []State {
	if len(prototypeIncludes) == 0 {
		return allStates
	}

	return append(allStates, State{
		ID:            StateID(produceStateID("prototype")),
		Label:         "prototype",
		IsRootContext: false,
		Includes:      dedupeStateIDs(prototypeIncludes),
	})
}

func buildIncludesWhitelist[TToken comparable](
	validTokens map[TToken]struct{},
	closeTok TToken,
	tokensInUse []TToken,
	getID func(TToken) StateID,
) []StateID {
	var includes []StateID
	for _, tok := range tokensInUse {
		if _, ok := validTokens[tok]; ok && tok != closeTok {
			includes = append(includes, getID(tok))
		}
	}
	return includes
}

// dedupeStateIDs returns a slice with duplicate StateIDs removed; first occurrence is kept, order preserved.
func dedupeStateIDs(includes []StateID) []StateID {
	return dedupeSlice(includes)
}

/*
InvalidFallbackRule returns a StateRule that matches non-whitespace (\S+) with the invalid.illegal scope. Used as the last rule in a state to catch unexpected tokens. id and scopeExtension are used for the rule ID and scope.
*/
func InvalidFallbackRule(id StateRuleID, scopeExtension string) StateRule {
	return StateRule{
		ID:       id,
		Label:    invalidFallbackLabel,
		RegEx:    `\S+`,
		Scope:    getScopeString([]string{invalidFallbackBaseScope}, scopeExtension),
		Action:   ACTION_MATCH,
		Priority: PriorityFallback,
	}
}

func getScopeString(baseScopes []string, scopeExtension string) string {
	return formatting.FormatStringSlice(baseScopes, formatting.FormatSliceOptions[string]{
		Separator: " ",
		FormatItem: func(_ int, value string) string {
			return formatIndividualScopeString(value, scopeExtension)
		},
	})
}

func formatIndividualScopeString(baseScope string, scopeExtension string) string {
	return fmt.Sprintf("%s%s", baseScope, scopeExtension)
}

func produceStateID(stateLabel string) uint64 {
	b := bytes.StringSliceToBytes([]string{stateLabel}, ';')
	return hash.XXH3HasherHash64(xxh3Hasher, b)
}

func sanitizeContextName(name string) string {
	var sb strings.Builder
	sb.Grow(len(name))
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	return sb.String()
}

func tokenToStateID[TToken, TTokenRole comparable](config *PushDownAutomatonIRConfiguration[TToken, TTokenRole], tok TToken) StateID {
	return StateID(produceStateID(sanitizeContextName(config.formatter(tok))))
}

// ------------------------------------------------------------------ SEQUENCE DFA COMPILATION

func hasNodeOverrides[TToken, TTokenRole comparable](config *PushDownAutomatonIRConfiguration[TToken, TTokenRole], node *syntaxa.Grammar[TToken]) bool {
	if node == nil || node.Kind != syntaxa.GConcat {
		return false
	}

	// 1. If the concat itself has a meta-scope, it MUST be a construct state chain.
	if oc, exists := config.nodeOverrides[node.GrammarLabel]; exists && oc.HasMetaScope() {
		return true
	}

	var found bool
	for _, child := range node.Children {
		if child == nil {
			continue
		}
		_ = child.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
			if n.Kind == syntaxa.GNest || n.Kind == syntaxa.GConcat {
				return true, false
			}
			if n.Kind == syntaxa.GToken {
				if oc, exists := config.nodeOverrides[n.GrammarLabel]; exists && oc.HasAnyScope() {
					found = true
					return true, true
				}
			}
			return false, false
		})
		if found {
			return true
		}
	}

	return false
}

func resolveScopeForTokenNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	node *syntaxa.Grammar[TToken],
) []string {
	scope := []string{config.scopeProvider(node.Token)}
	if oc, ok := config.nodeOverrides[node.GrammarLabel]; ok {
		if specificScopes, hasSpecific := oc.TokenScopes[node.Token]; hasSpecific {
			scope = specificScopes
		} else if len(oc.Scopes) > 0 {
			scope = oc.Scopes
		}
	}
	return scope
}

func generateRulesForNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	node *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
	tokenPriorityMap map[TToken]int,
	nextID StateID,
	stateLabel string,
	action RuleAction,
) []StateRule {
	if node == nil {
		return nil
	}

	var rules []StateRule
	_ = node.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		switch editorIRRole(n) {
		case EditorIRRoleToken:
			if _, hasTokOverride := config.overrides[n.Token]; hasTokOverride {
				return true, false
			}
			scope := resolveScopeForTokenNode(config, n)
			regex, _ := tokenPatternMap[n.Token].ToRegEx()
			lexerPriority := tokenPriorityMap[n.Token]
			rulePriority := PriorityDefault - lexerPriority
			rules = append(rules, StateRule{
				ID:           StateRuleID(produceStateID(fmt.Sprintf("%s_match_%s_%v", stateLabel, n.GrammarLabel, n.Token))),
				Label:        fmt.Sprintf("%s_match_%s", stateLabel, n.GrammarLabel),
				RegEx:        regex,
				Scope:        getScopeString(scope, config.scopeExtension),
				Action:       action,
				ActionTarget: nextID,
				Priority:     rulePriority,
			})
			return true, false
		case EditorIRRoleSegment:
			return false, false
		case EditorIRRoleNest, EditorIRRoleEpsilon:
			return true, false
		default:
			// Unknown role (e.g. new GrammarKind not yet in editorIRRole): skip to avoid wrong rules.
			return true, false
		}
	})
	return rules
}

/*
IRPlan is the result of a single pass over the grammar: it records which GConcat nodes are "construct" contexts (have node overrides), which GToken nodes have scope overrides, and the sequence triggers (GToken then GNest pairs). Used by the IR builder to inject override states, construct state chains, and trigger IDs.
*/
type IRPlan[TToken comparable] struct {
	ConstructConacts map[syntaxa.GrammarLabel]struct{}
	OverrideTokens   map[syntaxa.GrammarLabel]struct{}
	SeqTriggers      map[StateID]seqTriggerSpec[TToken]
}

func (p *IRPlan[TToken]) containsConstruct(id syntaxa.GrammarLabel) bool {
	_, ok := p.ConstructConacts[id]
	return ok
}

type seqTriggerSpec[TToken comparable] struct {
	ID         StateID
	Label      string
	Token      TToken
	TargetNest syntaxa.GrammarLabel
}

/*
BuildIRPlan walks the grammar tree and builds an IRPlan: ConstructConacts (GConcat nodes with node overrides), OverrideTokens (GToken nodes with scope override), and SeqTriggers (token–nest pairs from sequence nodes via GrammarSequenceTokenNestPairs). config.nodeOverrides and config.overrides drive which nodes are considered. root must be the entry grammar (e.g. grammarPackage.Rules[grammarPackage.EntryRule]). analysis may be nil; when set, sequence triggers use First(prev) for non-GToken prev.
*/
func BuildIRPlan[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	analysis *syntaxa.GrammarAnalysis[TToken],
	root *syntaxa.Grammar[TToken],
) *IRPlan[TToken] {
	plan := &IRPlan[TToken]{
		ConstructConacts: make(map[syntaxa.GrammarLabel]struct{}),
		OverrideTokens:   make(map[syntaxa.GrammarLabel]struct{}),
		SeqTriggers:      make(map[StateID]seqTriggerSpec[TToken]),
	}

	if root == nil {
		return plan
	}

	_ = root.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			plan.ConstructConacts[n.GrammarLabel] = struct{}{}
		}

		if n.Kind == syntaxa.GToken {
			if oc, ok := config.nodeOverrides[n.GrammarLabel]; ok && oc.HasAnyScope() {
				plan.OverrideTokens[n.GrammarLabel] = struct{}{}
			}
		}

		if n.Kind == syntaxa.GConcat {
			for _, p := range syntaxa.GrammarSequenceTokenNestPairs(analysis, n) {
				id := deriveTriggerID(config.formatter(p.Token), p.NestID)
				if _, exists := plan.SeqTriggers[id]; !exists {
					plan.SeqTriggers[id] = seqTriggerSpec[TToken]{
						ID:         id,
						Label:      fmt.Sprintf("seq_trigger_%s", sanitizeContextName(string(p.NestID))),
						Token:      p.Token,
						TargetNest: p.NestID,
					}
				}
			}
		}
		return false, false
	})
	return plan
}

func buildBodyStatePlanned[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	bodyStateID StateID,
	tokensInUse []TToken,
) State {
	bodyGrammarForIncludes := syntaxa.GrammarNestBody(nest.Node)
	if bodyGrammarForIncludes == nil {
		bodyGrammarForIncludes = nest.Node
	}
	validTokens, overrideIDs, triggerIDs := extractIncludes(config, plan, analysis, bodyGrammarForIncludes, false)

	var includes []StateID
	includes = append(includes, triggerIDs...)
	includes = append(includes, overrideIDs...)

	baseIncludes := buildIncludesWhitelist(validTokens, nest.Close, tokensInUse, func(tok TToken) StateID {
		return tokenToStateID(config, tok)
	})
	includes = append(includes, baseIncludes...)

	closeRegex, _ := tokenPatternMap[nest.Close].ToRegEx()
	metaScopeBase := fmt.Sprintf("meta.block.%s", strings.ToLower(nestLabel))

	return State{
		ID:        bodyStateID,
		Label:     nestLabel,
		MetaScope: getScopeString([]string{metaScopeBase}, config.scopeExtension),
		Includes:  dedupeStateIDs(includes),
		Rules: []StateRule{
			{
				ID:     StateRuleID(produceStateID(nestLabel + "_close")),
				Label:  nestLabel + "_close",
				RegEx:  closeRegex,
				Scope:  getScopeString([]string{config.scopeProvider(nest.Close)}, config.scopeExtension),
				Action: ACTION_POP,
			},
			InvalidFallbackRule(StateRuleID(produceStateID(nestLabel+"_invalid")), config.scopeExtension),
		},
	}
}

func injectNestStatesPlanned[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind, TLexerState comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	allStates []State,
	grammarPackage syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	tokenPatternMap map[TToken]Pattern,
	tokensInUse []TToken,
) ([]State, map[syntaxa.GrammarLabel]StateID, map[syntaxa.GrammarLabel]StateID) {
	openTokenCounts := syntaxa.NestSpecsOpenTokenCounts(grammarPackage.Nests)

	nestRegistry := make(map[syntaxa.GrammarLabel]StateID)
	nestBodyRegistry := make(map[syntaxa.GrammarLabel]StateID)

	for _, nest := range grammarPackage.Nests {
		nestLabel := sanitizeContextName(string(nest.OwnerRule))
		isUniqueToken := openTokenCounts[nest.Open] == 1
		openStateLabel := sanitizeContextName(config.formatter(nest.Open))
		openStateID := StateID(produceStateID(openStateLabel))

		if customStates, entryID, handled := tryApplyNestOverride(config, nest, nestLabel, tokenPatternMap); handled {
			nestBodyRegistry[nest.ID] = entryID
			if isUniqueToken {
				mutateStateAction(allStates, openStateID, ACTION_PUSH, entryID)
			} else {
				nestRegistry[nest.ID] = entryID
			}
			allStates = append(allStates, customStates...)
			continue
		}

		bodyStateID := StateID(produceStateID(nestLabel))
		nestBodyRegistry[nest.ID] = bodyStateID

		bodyState := buildBodyStatePlanned(config, plan, grammarPackage.Analysis, nest, nestLabel, tokenPatternMap, bodyStateID, tokensInUse)
		allStates = append(allStates, bodyState)

		if isUniqueToken {
			mutateStateAction(allStates, openStateID, ACTION_PUSH, bodyStateID)
		} else {
			entryGrammar := grammarPackage.Grammars[grammarPackage.EntryRule]

			expectAnalysis := collectExpectStateIncludes(config, plan, grammarPackage.Analysis, entryGrammar, nest.ID)

			expectStateID := StateID(produceStateID(nestLabel + "_expect"))
			expectState := buildExpectState(config, nest, nestLabel, tokenPatternMap, expectStateID, bodyStateID, expectAnalysis, tokensInUse)

			nestRegistry[nest.ID] = expectStateID
			allStates = append(allStates, expectState)
		}
	}

	return allStates, nestRegistry, nestBodyRegistry
}

func injectMainStatePlanned[TToken, TTokenRole comparable](
	allStates []State,
	scopeExtension string,
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	entryNode *syntaxa.Grammar[TToken],
) []State {
	_, overrideIDs, triggerIDs := extractIncludes(config, plan, analysis, entryNode, true)

	var rootIncludes []StateID
	rootIncludes = append(rootIncludes, triggerIDs...)
	rootIncludes = append(rootIncludes, overrideIDs...)

	for _, st := range allStates {
		if st.IsRootContext {
			rootIncludes = append(rootIncludes, st.ID)
		}
	}

	return append(allStates, State{
		ID:       StateID(produceStateID("main")),
		Label:    "main",
		Includes: dedupeStateIDs(rootIncludes),
		Rules: []StateRule{
			InvalidFallbackRule(StateRuleID(produceStateID("main_invalid")), scopeExtension),
		},
	})
}

// ------------------------------------------------------------------ GRAMMAR UTILITIES

func buildLookaheadForTokens[TToken comparable](toks map[TToken]struct{}, tokenPatternMap map[TToken]Pattern) (string, bool) {
	if len(toks) == 0 {
		return "", false
	}
	var parts []string
	for t := range toks {
		r, err := tokenPatternMap[t].ToRegEx()
		if err == nil && r != "" {
			parts = append(parts, fmt.Sprintf("(?:%s)", r))
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	return fmt.Sprintf(`(?=\s*(?:%s))`, strings.Join(parts, "|")), true
}

func optimizeAutomaton(allStates []State) []State {
	stateMap := mapStatesByID(allStates)
	roots := getRootStateIDs(allStates)
	reachable := markReachableStates(stateMap, roots)

	return sweepUnreachable(allStates, reachable)
}

func mapStatesByID(states []State) map[StateID]State {
	m := make(map[StateID]State, len(states))
	for _, s := range states {
		m[s.ID] = s
	}
	return m
}

func getRootStateIDs(states []State) []StateID {
	var roots []StateID
	for _, s := range states {
		if s.Label == "main" || s.Label == "prototype" {
			roots = append(roots, s.ID)
		}
	}
	return roots
}

func markReachableStates(stateMap map[StateID]State, initialRoots []StateID) map[StateID]bool {
	reachable := make(map[StateID]bool)
	queue := append([]StateID(nil), initialRoots...)

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if reachable[current] {
			continue
		}

		reachable[current] = true

		state, exists := stateMap[current]
		if !exists {
			continue
		}

		queue = append(queue, getOutboundEdges(state)...)
	}

	return reachable
}

func getOutboundEdges(state State) []StateID {
	edges := make([]StateID, 0, len(state.Includes)+len(state.Rules))
	edges = append(edges, state.Includes...)

	for _, r := range state.Rules {
		if r.Action == ACTION_PUSH || r.Action == ACTION_SET {
			edges = append(edges, r.ActionTarget)
		}
	}

	return edges
}

func sweepUnreachable(states []State, reachable map[StateID]bool) []State {
	var pruned []State
	for _, s := range states {
		if reachable[s.ID] {
			pruned = append(pruned, s)
		}
	}
	return pruned
}

func appendStrictSequenceBailout(rules []StateRule, stateLabel string, recoveryStateID StateID) []StateRule {
	bailoutID := StateRuleID(produceStateID(stateLabel + "_sequence_bailout"))

	bailoutRule := StateRule{
		ID:           bailoutID,
		Label:        "sequence_bailout",
		RegEx:        `(?=\S)`,
		Action:       ACTION_SET,
		ActionTarget: recoveryStateID,
		Priority:     PriorityFallback,
	}

	return append(rules, bailoutRule)
}
