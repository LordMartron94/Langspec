package editor

import (
	"autarch/pattern"
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
type OverrideConfig struct {
	Scopes    []string
	MetaScope string
}

func (c OverrideConfig) HasScope() bool     { return len(c.Scopes) > 0 }
func (c OverrideConfig) HasMetaScope() bool { return c.MetaScope != "" }
func (c OverrideConfig) HasAny() bool       { return c.HasScope() || c.HasMetaScope() }

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
	nodeOverrides        map[syntaxa.GrammarID]OverrideConfig
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
		nodeOverrides:        make(map[syntaxa.GrammarID]OverrideConfig),
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
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverride(ruleID syntaxa.GrammarID, fn NestOverrideFunc[TToken]) {
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
AddNodeOverride attaches scope and/or meta-scope to a grammar node by GrammarID. Node must be a GToken (scope) or GConcat (meta-scope) for the override to apply.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeOverride(nodeID syntaxa.GrammarID, config OverrideConfig) {
	c.nodeOverrides[nodeID] = config
}

/*
AddNodeScopeOverride is a convenience for AddNodeOverride with only Scope set. Use for token nodes that need a custom scope.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeScopeOverride(nodeID syntaxa.GrammarID, scopes ...string) {
	c.AddNodeOverride(nodeID, OverrideConfig{Scopes: scopes})
}

/*
AddPrototypeTokenRoles marks tokens with the given roles as prototype contexts: they are included in the root prototype state so they can be entered from the main context.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddPrototypeTokenRoles(tokenRoles ...TTokenRole) {
	c.prototypeTokenRoles = append(c.prototypeTokenRoles, tokenRoles...)
}

// ------------------------------------------------------------------ IR ORCHESTRATOR

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
It builds base states from tokens and delimited rules, injects node overrides and construct states from the grammar, injects nest states and sequence triggers, adds the prototype and main states, and returns the complete IR. grammarPackage.Analysis and grammarPackage.TokenNodeByID must be populated (use Grammar.ProducePackage).
*/
func PushDownAutomatonIRCreate[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TToken],
) *PushDownAutomatonIR {
	entryGrammar := grammarPackage.Rules[grammarPackage.EntryRule]

	plan := BuildIRPlan(config, entryGrammar)
	tokensInUse, tokenPatternMap, prototypeTokens := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)
	delimitedMap := extractDelimitedRules(lexingRuleSet)

	rootTokens, _, _ := extractIncludes(config, plan, entryGrammar, true)
	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, prototypeTokens, rootTokens, delimitedMap)

	allStates = injectPlannedNodeStates(config, plan, grammarPackage.Analysis, grammarPackage.TokenNodeByID, allStates, entryGrammar, tokenPatternMap)
	allStates, nestRegistry := injectNestStatesPlanned(config, plan, allStates, grammarPackage, tokenPatternMap)
	allStates = injectPlannedSequenceTriggers(config, plan, allStates, nestRegistry, tokenPatternMap)
	allStates = injectPrototypeState(allStates, prototypeIncludes)
	allStates = injectMainStatePlanned(allStates, config.scopeExtension, config, plan, entryGrammar)

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
) ([]TToken, map[TToken]Pattern, map[TToken]bool) {
	var tokensInUse []TToken
	tokenPatternMap := make(map[TToken]Pattern)
	prototypeTokens := make(map[TToken]bool)

	for _, lexerRule := range lexingRuleSet.GetRules() {
		tokensInUse = append(tokensInUse, lexerRule.Token)
		tokenPatternMap[lexerRule.Token] = lexerRule.Pattern

		if slices.Contains(prototypeTokenRoles, lexerRule.Role) {
			prototypeTokens[lexerRule.Token] = true
		}
	}
	return tokensInUse, tokenPatternMap, prototypeTokens
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
	prototypeTokens map[TToken]bool,
	rootTokens map[TToken]struct{},
	delimitedMap map[TToken]DelimitedRuleRegex[TToken],
) ([]State, []StateID) {
	var allStates []State
	var prototypeIncludes []StateID

	for _, token := range tokensInUse {
		state, extraStates, isProto := buildSingleBaseState(config, token, tokenPatternMap, prototypeTokens, rootTokens, delimitedMap)
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
	prototypeTokens map[TToken]bool,
	rootTokens map[TToken]struct{},
	delimitedMap map[TToken]DelimitedRuleRegex[TToken],
) (State, []State, bool) {
	label := sanitizeContextName(config.formatter(token))
	id := StateID(produceStateID(label))
	baseScope := config.scopeProvider(token)

	var mainRule StateRule
	var extraStates []State

	if delimited, hasDelimited := delimitedMap[token]; hasDelimited {
		ctx := buildTokenOverrideContext(id, label, tokenPatternMap[token], baseScope, config.scopeExtension)
		mainRule, extraStates = TokenOverrideDelimitedRegion(ctx, delimited.OpenRegex, delimited.CloseRegex, "punctuation.definition.comment.begin", baseScope, "punctuation.definition.comment.end")
	} else if overrideFn, exists := config.overrides[token]; exists {
		ctx := buildTokenOverrideContext(id, label, tokenPatternMap[token], baseScope, config.scopeExtension)
		mainRule, extraStates = overrideFn(ctx)
	} else {
		defaultRegex, _ := tokenPatternMap[token].ToRegEx()
		mainRule = StateRule{
			ID:     StateRuleID(id),
			Label:  label,
			Action: ACTION_MATCH,
			Scope:  getScopeString([]string{baseScope}, config.scopeExtension),
			RegEx:  defaultRegex,
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
) State {
	openRegex, _ := tokenPatternMap[nest.Open].ToRegEx()

	return State{
		ID:            expectStateID,
		Label:         nestLabel + "_expect",
		IsRootContext: false,
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

// constructBuildEnv holds shared context for the construct-state pipeline (config, plan, analysis, token patterns).
type constructBuildEnv[TToken, TTokenRole comparable] struct {
	Config          *PushDownAutomatonIRConfiguration[TToken, TTokenRole]
	Plan            *IRPlan[TToken]
	Analysis        *syntaxa.GrammarAnalysis[TToken]
	TokenPatternMap map[TToken]Pattern
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

func nodeOverrideStateID(grammarID syntaxa.GrammarID) StateID {
	return StateID(produceStateID(fmt.Sprintf("node_override_%s", sanitizeContextName(string(grammarID)))))
}

func nodeConstructEntryStateID(concatID syntaxa.GrammarID) StateID {
	return StateID(produceStateID(fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatID)))))
}

func buildConstructStateChain[TToken, TTokenRole comparable](
	env *constructBuildEnv[TToken, TTokenRole],
	concatNode *syntaxa.Grammar[TToken],
	metaScope string,
	inheritedSyncTokens []TToken,
	isRepeating bool,
) []State {
	baseLabel := fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatNode.GrammarID)))
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

// dedupeSyncTokens returns a slice with duplicate tokens removed (first occurrence kept).
func dedupeSyncTokens[TToken comparable](tokens []TToken) []TToken {
	seen := make(map[TToken]struct{}, len(tokens))
	out := make([]TToken, 0, len(tokens))
	for _, t := range tokens {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
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
	id := stepID(baseLabel, chainCtx.ConcatNode.GrammarID, index, stepCount)
	isToken := syntaxa.GrammarIsTokenNode(child)

	config := chainCtx.Env.Config
	var rules []StateRule
	includes := buildIncludesForNode(config, child)

	if isToken {
		rules, includes = handleTokenConstructStep(chainCtx, child, index, lbl, includes)
	} else {
		rules, includes = handleSegmentConstructStep(chainCtx, index, lbl, includes)
	}

	if index > 0 {
		rules = appendStrictSequenceBailout(rules, lbl, chainCtx.RecoveryStateID)
	}

	st := State{
		ID:       id,
		Label:    lbl,
		Rules:    rules,
		Includes: includes,
	}
	if index > 0 && chainCtx.MetaScope != "" {
		st.MetaScope = getScopeString([]string{chainCtx.MetaScope}, config.scopeExtension)
	}
	return st
}

func handleTokenConstructStep[TToken, TTokenRole comparable](
	chainCtx *constructChainCtx[TToken, TTokenRole],
	child *syntaxa.Grammar[TToken],
	index int,
	lbl string,
	includes []StateID,
) ([]StateRule, []StateID) {
	env := chainCtx.Env
	action, nextID := getTransition(baseLabelForTransition(chainCtx.ConcatNode.GrammarID), index, index+1, chainCtx.StepCount, chainCtx.IsRepeating, chainCtx.RecoveryStateID)

	if _, hasTokOverride := env.Config.overrides[child.Token]; hasTokOverride {
		return buildLookaheadRules(chainCtx, index, action, nextID), includes
	}
	return generateRulesForNode(env.Config, child, env.TokenPatternMap, nextID, lbl, action), nil
}

func handleSegmentConstructStep[TToken, TTokenRole comparable](
	chainCtx *constructChainCtx[TToken, TTokenRole],
	index int,
	lbl string,
	includes []StateID,
) ([]StateRule, []StateID) {
	action, nextID := getTransition(baseLabelForTransition(chainCtx.ConcatNode.GrammarID), index, index+1, chainCtx.StepCount, chainCtx.IsRepeating, chainCtx.RecoveryStateID)
	rules := buildLookaheadRules(chainCtx, index, action, nextID)
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

func stepID(baseLabel string, grammarID syntaxa.GrammarID, index, stepCount int) StateID {
	if index == 0 {
		return nodeConstructEntryStateID(grammarID)
	}
	return StateID(produceStateID(stepLabel(baseLabel, index, stepCount)))
}

func baseLabelForTransition(grammarID syntaxa.GrammarID) string {
	return fmt.Sprintf("node_construct_%s", sanitizeContextName(string(grammarID)))
}

func buildIncludesForNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	node *syntaxa.Grammar[TToken],
) []StateID {
	validTokens, overrideIDs, triggerIDs := extractIncludes(config, nil, node, false)

	if node != nil && node.Kind == syntaxa.GNest && node.OpenToken != nil {
		validTokens[*node.OpenToken] = struct{}{}
	}

	var includes []StateID
	includes = append(includes, triggerIDs...)
	includes = append(includes, overrideIDs...)
	includes = append(includes, buildIncludesWhitelist(validTokens, *new(TToken), *new(TToken), func(tok TToken) StateID {
		return StateID(produceStateID(sanitizeContextName(config.formatter(tok))))
	})...)
	return includes
}

func injectPlannedNodeStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	tokenNodeByID map[syntaxa.GrammarID]*syntaxa.Grammar[TToken],
	allStates []State,
	grammarNode *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
) []State {
	for gid := range plan.OverrideTokens {
		tok := tokenNodeByID[gid]
		if tok == nil {
			continue
		}

		oc := config.nodeOverrides[gid]
		stateID := nodeOverrideStateID(gid)
		regex, _ := tokenPatternMap[tok.Token].ToRegEx()

		allStates = append(allStates, State{
			ID:    stateID,
			Label: fmt.Sprintf("node_override_%s", sanitizeContextName(string(gid))),
			Rules: []StateRule{{
				ID:     StateRuleID(stateID),
				Label:  fmt.Sprintf("node_override_%s_match", sanitizeContextName(string(gid))),
				Action: ACTION_MATCH,
				Scope:  getScopeString(oc.Scopes, config.scopeExtension),
				RegEx:  regex,
			}},
		})
	}

	env := &constructBuildEnv[TToken, TTokenRole]{
		Config:          config,
		Plan:            plan,
		Analysis:        analysis,
		TokenPatternMap: tokenPatternMap,
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
			if _, ok := plan.ConstructConacts[node.GrammarID]; ok {
				var metaScope string
				if oc, ok2 := config.nodeOverrides[node.GrammarID]; ok2 && oc.HasMetaScope() {
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
	nestRegistry map[syntaxa.GrammarID]StateID,
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

func deriveTriggerID(tokenLabel string, targetID syntaxa.GrammarID) StateID {
	cleanToken := sanitizeContextName(tokenLabel)
	cleanTarget := sanitizeContextName(string(targetID))
	return StateID(produceStateID(fmt.Sprintf("seq_trigger_%s_to_%s", cleanToken, cleanTarget)))
}

// ------------------------------------------------------------------ CONTEXT EXTRACTION & INJECTION

type includeAnalysis[TToken comparable] struct {
	ValidTokens  map[TToken]struct{}
	TriggerIDs   []StateID
	OverrideIDs  []StateID
	ConstructIDs []StateID
}

func extractIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken], // Optional: Pass nil to ignore plan checks
	contextRoot *syntaxa.Grammar[TToken],
	isRoot bool,
) (map[TToken]struct{}, []StateID, []StateID) {
	a := traverseIncludes(config, plan, contextRoot, isRoot)

	var overrides []StateID
	overrides = append(overrides, a.ConstructIDs...)
	overrides = append(overrides, a.OverrideIDs...)

	return a.ValidTokens, overrides, a.TriggerIDs
}

func traverseIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	root *syntaxa.Grammar[TToken],
	isRoot bool,
) includeAnalysis[TToken] {
	out := includeAnalysis[TToken]{
		ValidTokens: make(map[TToken]struct{}),
	}

	if root != nil && root.Kind == syntaxa.GNest && root.OpenToken != nil {
		out.ValidTokens[*root.OpenToken] = struct{}{}
	}

	if root == nil {
		return out
	}

	seenTriggers := make(map[StateID]bool)
	seenOverrides := make(map[StateID]bool)
	seenConstructs := make(map[StateID]bool)

	_ = root.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			if plan == nil || plan.containsConstruct(n.GrammarID) {
				cid := nodeConstructEntryStateID(n.GrammarID)
				if !seenConstructs[cid] {
					seenConstructs[cid] = true
					out.ConstructIDs = append(out.ConstructIDs, cid)
				}
				return true, false
			}
		}

		if n.Kind == syntaxa.GConcat {
			for _, p := range syntaxa.GrammarConcatTokenNestPairs(n) {
				id := deriveTriggerID(config.formatter(p.Token), p.NestID)
				if !seenTriggers[id] {
					seenTriggers[id] = true
					out.TriggerIDs = append(out.TriggerIDs, id)
				}
			}
		}

		switch n.Kind {
		case syntaxa.GToken:
			if oc, ok := config.nodeOverrides[n.GrammarID]; ok && oc.HasScope() {
				oid := nodeOverrideStateID(n.GrammarID)
				if !seenOverrides[oid] {
					seenOverrides[oid] = true
					out.OverrideIDs = append(out.OverrideIDs, oid)
				}
			} else {
				out.ValidTokens[n.Token] = struct{}{}
			}

		case syntaxa.GNest:
			if isRoot || n != root {
				if n.OpenToken != nil {
					out.ValidTokens[*n.OpenToken] = struct{}{}
				}
				return true, false
			}
		}

		return false, false
	})
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
		Includes:      prototypeIncludes,
	})
}

func buildIncludesWhitelist[TToken comparable](
	validTokens map[TToken]struct{},
	openTok, closeTok TToken,
	getID func(TToken) StateID,
) []StateID {
	var includes []StateID
	for tok := range validTokens {
		if tok != openTok && tok != closeTok {
			includes = append(includes, getID(tok))
		}
	}
	return includes
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

// ------------------------------------------------------------------ SEQUENCE DFA COMPILATION

func hasNodeOverrides[TToken, TTokenRole comparable](config *PushDownAutomatonIRConfiguration[TToken, TTokenRole], node *syntaxa.Grammar[TToken]) bool {
	if node == nil {
		return false
	}

	var found bool
	_ = node.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		if n.Kind == syntaxa.GNest {
			return true, false
		}
		if oc, exists := config.nodeOverrides[n.GrammarID]; exists && oc.HasAny() {
			if (n.Kind == syntaxa.GToken && oc.HasScope()) || (n.Kind == syntaxa.GConcat && oc.HasMetaScope()) {
				found = true
				return true, true
			}
		}
		return false, false
	})
	return found
}

func generateRulesForNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	node *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
	nextID StateID,
	stateLabel string,
	action RuleAction,
) []StateRule {
	if node == nil {
		return nil
	}

	var rules []StateRule
	_ = node.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		switch n.Kind {
		case syntaxa.GToken:
			if _, hasTokOverride := config.overrides[n.Token]; hasTokOverride {
				return true, false
			}
			scope := []string{config.scopeProvider(n.Token)}
			if oc, ok := config.nodeOverrides[n.GrammarID]; ok && oc.HasScope() {
				scope = oc.Scopes
			}
			regex, _ := tokenPatternMap[n.Token].ToRegEx()
			rules = append(rules, StateRule{
				ID:           StateRuleID(produceStateID(fmt.Sprintf("%s_match_%s_%v", stateLabel, n.GrammarID, n.Token))),
				Label:        fmt.Sprintf("%s_match_%s", stateLabel, n.GrammarID),
				RegEx:        regex,
				Scope:        getScopeString(scope, config.scopeExtension),
				Action:       action,
				ActionTarget: nextID,
			})
			return true, false
		case syntaxa.GChoice, syntaxa.GOptional, syntaxa.GConcat, syntaxa.GRepeat:
			return false, false
		default:
			return true, false
		}
	})
	return rules
}

/*
IRPlan is the result of a single pass over the grammar: it records which GConcat nodes are "construct" contexts (have node overrides), which GToken nodes have scope overrides, and the sequence triggers (GToken then GNest pairs). Used by the IR builder to inject override states, construct state chains, and trigger IDs.
*/
type IRPlan[TToken comparable] struct {
	ConstructConacts map[syntaxa.GrammarID]struct{}
	OverrideTokens   map[syntaxa.GrammarID]struct{}
	SeqTriggers      map[StateID]seqTriggerSpec[TToken]
}

func (p *IRPlan[TToken]) containsConstruct(id syntaxa.GrammarID) bool {
	_, ok := p.ConstructConacts[id]
	return ok
}

type seqTriggerSpec[TToken comparable] struct {
	ID         StateID
	Label      string
	Token      TToken
	TargetNest syntaxa.GrammarID
}

/*
BuildIRPlan walks the grammar tree and builds an IRPlan: ConstructConacts (GConcat nodes with node overrides), OverrideTokens (GToken nodes with scope override), and SeqTriggers (token–nest pairs from GConcat children). config.nodeOverrides and config.overrides drive which nodes are considered. root must be the entry grammar (e.g. grammarPackage.Rules[grammarPackage.EntryRule]).
*/
func BuildIRPlan[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	root *syntaxa.Grammar[TToken],
) *IRPlan[TToken] {
	plan := &IRPlan[TToken]{
		ConstructConacts: make(map[syntaxa.GrammarID]struct{}),
		OverrideTokens:   make(map[syntaxa.GrammarID]struct{}),
		SeqTriggers:      make(map[StateID]seqTriggerSpec[TToken]),
	}

	if root == nil {
		return plan
	}

	_ = root.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			plan.ConstructConacts[n.GrammarID] = struct{}{}
		}

		if n.Kind == syntaxa.GToken {
			if oc, ok := config.nodeOverrides[n.GrammarID]; ok && oc.HasScope() {
				plan.OverrideTokens[n.GrammarID] = struct{}{}
			}
		}

		if n.Kind == syntaxa.GConcat {
			for _, p := range syntaxa.GrammarConcatTokenNestPairs(n) {
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
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	bodyStateID StateID,
) State {
	validTokens, overrideIDs, triggerIDs := extractIncludes(config, plan, nest.Node.Children[0], false)

	var includes []StateID
	includes = append(includes, triggerIDs...)
	includes = append(includes, overrideIDs...)

	baseIncludes := buildIncludesWhitelist(validTokens, nest.Open, nest.Close, func(tok TToken) StateID {
		return StateID(produceStateID(sanitizeContextName(config.formatter(tok))))
	})
	includes = append(includes, baseIncludes...)

	closeRegex, _ := tokenPatternMap[nest.Close].ToRegEx()
	metaScopeBase := fmt.Sprintf("meta.block.%s", strings.ToLower(nestLabel))

	return State{
		ID:        bodyStateID,
		Label:     nestLabel + "_body",
		MetaScope: getScopeString([]string{metaScopeBase}, config.scopeExtension),
		Includes:  includes,
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

func injectNestStatesPlanned[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	allStates []State,
	grammarPackage syntaxa.GrammarPackage[TToken],
	tokenPatternMap map[TToken]Pattern,
) ([]State, map[syntaxa.GrammarID]StateID) {
	openTokenCounts := syntaxa.NestSpecsOpenTokenCounts(grammarPackage.Nests)
	nestRegistry := make(map[syntaxa.GrammarID]StateID)

	for _, nest := range grammarPackage.Nests {
		nestLabel := sanitizeContextName(string(nest.OwnerRule))
		isUniqueToken := openTokenCounts[nest.Open] == 1
		openStateID := StateID(produceStateID(sanitizeContextName(config.formatter(nest.Open))))

		if customStates, entryID, handled := tryApplyNestOverride(config, nest, nestLabel, tokenPatternMap); handled {
			if isUniqueToken {
				mutateStateAction(allStates, openStateID, ACTION_PUSH, entryID)
			} else {
				nestRegistry[nest.ID] = entryID
			}
			allStates = append(allStates, customStates...)
			continue
		}

		bodyStateID := StateID(produceStateID(nestLabel + "_body"))
		bodyState := buildBodyStatePlanned(config, plan, nest, nestLabel, tokenPatternMap, bodyStateID)
		allStates = append(allStates, bodyState)

		if isUniqueToken {
			mutateStateAction(allStates, openStateID, ACTION_PUSH, bodyStateID)
			continue
		}

		expectStateID := StateID(produceStateID(nestLabel + "_expect"))
		expectState := buildExpectState(config, nest, nestLabel, tokenPatternMap, expectStateID, bodyStateID)

		nestRegistry[nest.ID] = expectStateID
		allStates = append(allStates, expectState)
	}

	return allStates, nestRegistry
}

func injectMainStatePlanned[TToken, TTokenRole comparable](
	allStates []State,
	scopeExtension string,
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	entryNode *syntaxa.Grammar[TToken],
) []State {
	_, overrideIDs, triggerIDs := extractIncludes(config, plan, entryNode, true)

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
		Includes: rootIncludes,
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
