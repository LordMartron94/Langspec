package editor

import (
	"autarch/pattern"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"lexarch"
	"slices"
	"strings"
	"syntaxa"
)

var xxh3Hasher = hash.XXH3HasherCreateWithSeed(42)

// ------------------------------------------------------------------ TYPES & CONSTANTS

type Pattern = pattern.RegulaAST[rune]
type StateID uint64
type StateRuleID uint64

//go:generate stringer -type RuleAction
type RuleAction uint8

const (
	ACTION_PUSH RuleAction = iota
	ACTION_POP
	ACTION_SET
	ACTION_MATCH
	ACTION_NONE
	ACTION_EMBED
)

const (
	invalidFallbackLabel     = "invalid_fallback"
	invalidFallbackBaseScope = "invalid.illegal.unexpected-token"
)

type State struct {
	ID            StateID
	Label         string
	MetaScope     string
	Rules         []StateRule
	Includes      []StateID
	IsRootContext bool
	OmitPrototype bool
}

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
}

type ScopeProvider[TToken any] func(item TToken) string
type TokenFormatter[TToken any] func(token TToken) string

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

func (ctx *TokenOverrideContext) ApplyScope(scope string) string {
	return getScopeString(scope, ctx.ScopeExtension)
}

type TokenOverrideFunc func(ctx *TokenOverrideContext) (mainRule StateRule, extraStates []State)

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

func (ctx *NestOverrideContext[TToken]) ApplyScope(scope string) string {
	return getScopeString(scope, ctx.ScopeExtension)
}

type NestOverrideFunc[TToken comparable] func(ctx *NestOverrideContext[TToken]) (entryStateID StateID, states []State)
type NestOverridePredicate[TToken comparable] func(nest *syntaxa.NestSpec[TToken]) bool

// ------------------------------------------------------------------ CONFIGURATION

type OverrideConfig struct {
	Scope     string
	MetaScope string
}

func (c OverrideConfig) HasScope() bool     { return c.Scope != "" }
func (c OverrideConfig) HasMetaScope() bool { return c.MetaScope != "" }
func (c OverrideConfig) HasAny() bool       { return c.HasScope() || c.HasMetaScope() }

type nestOverrideHandler[TToken comparable] struct {
	pred NestOverridePredicate[TToken]
	fn   NestOverrideFunc[TToken]
}

type PushDownAutomatonIRConfiguration[TToken, TTokenRole comparable] struct {
	scopeProvider        ScopeProvider[TToken]
	formatter            TokenFormatter[TToken]
	scopeExtension       string
	overrides            map[TToken]TokenOverrideFunc
	prototypeTokenRoles  []TTokenRole
	nestOverrideHandlers []nestOverrideHandler[TToken]
	nodeOverrides        map[syntaxa.GrammarID]OverrideConfig
}

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

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddOverride(token TToken, fn TokenOverrideFunc) {
	c.overrides[token] = fn
}

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverride(ruleID syntaxa.GrammarID, fn NestOverrideFunc[TToken]) {
	c.AddNestOverrideByPredicate(func(nest *syntaxa.NestSpec[TToken]) bool {
		return nest.OwnerRule == ruleID
	}, fn)
}

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverrideByPredicate(pred NestOverridePredicate[TToken], fn NestOverrideFunc[TToken]) {
	c.nestOverrideHandlers = append(c.nestOverrideHandlers, nestOverrideHandler[TToken]{pred: pred, fn: fn})
}

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeOverride(nodeID syntaxa.GrammarID, config OverrideConfig) {
	c.nodeOverrides[nodeID] = config
}

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeScopeOverride(nodeID syntaxa.GrammarID, scope string) {
	c.AddNodeOverride(nodeID, OverrideConfig{Scope: scope})
}

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddPrototypeTokenRoles(tokenRoles ...TTokenRole) {
	c.prototypeTokenRoles = append(c.prototypeTokenRoles, tokenRoles...)
}

// ------------------------------------------------------------------ IR ORCHESTRATOR

type PushDownAutomatonIR struct {
	LanguageName    string
	LanguageVersion string
	States          []State
	ScopeExtension  string
}

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

	return &PushDownAutomatonIR{
		LanguageName:    grammarPackage.Name,
		LanguageVersion: grammarPackage.Version,
		States:          allStates,
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
			Scope:  getScopeString(baseScope, config.scopeExtension),
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
				Scope:        getScopeString(config.scopeProvider(nest.Open), config.scopeExtension),
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

func nodeOverrideStateID(grammarID syntaxa.GrammarID) StateID {
	return StateID(produceStateID(fmt.Sprintf("node_override_%s", sanitizeContextName(string(grammarID)))))
}

func nodeConstructEntryStateID(concatID syntaxa.GrammarID) StateID {
	return StateID(produceStateID(fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatID)))))
}

func buildConstructStateChain[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	analysis *syntaxa.GrammarAnalysis[TToken],
	concatNode *syntaxa.Grammar[TToken],
	metaScope string,
	tokenPatternMap map[TToken]Pattern,
) []State {
	baseLabel := fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatNode.GrammarID)))
	stepCount := len(concatNode.Children)
	var states []State

	for i := 0; i < stepCount; i++ {
		states = append(states, buildConstructStep(config, analysis, concatNode, i, stepCount, baseLabel, metaScope, tokenPatternMap))
	}
	return states
}

func buildConstructStep[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	analysis *syntaxa.GrammarAnalysis[TToken],
	concatNode *syntaxa.Grammar[TToken],
	index, stepCount int,
	baseLabel, metaScope string,
	tokenPatternMap map[TToken]Pattern,
) State {
	child := concatNode.Children[index]
	lbl := stepLabel(baseLabel, index, stepCount)
	id := stepID(baseLabel, concatNode.GrammarID, index, stepCount)
	isToken := syntaxa.GrammarIsTokenNode(child)

	var rules []StateRule
	includes := buildIncludesForNode(config, child)

	if isToken {
		rules, includes = handleTokenConstructStep(config, analysis, concatNode, child, index, stepCount, lbl, includes, tokenPatternMap)
	} else {
		rules, includes = handleSegmentConstructStep(config, concatNode, index, stepCount, lbl, includes, tokenPatternMap)
	}

	st := State{
		ID:       id,
		Label:    lbl,
		Rules:    rules,
		Includes: includes,
	}
	if index > 0 && metaScope != "" {
		st.MetaScope = getScopeString(metaScope, config.scopeExtension)
	}
	return st
}

func handleTokenConstructStep[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	analysis *syntaxa.GrammarAnalysis[TToken],
	concatNode, child *syntaxa.Grammar[TToken],
	index, stepCount int,
	lbl string,
	includes []StateID,
	tokenPatternMap map[TToken]Pattern,
) ([]StateRule, []StateID) {
	action, nextID := getTransition(baseLabelForTransition(concatNode.GrammarID), index, index+1, stepCount)

	if _, hasTokOverride := config.overrides[child.Token]; hasTokOverride {
		return buildLookaheadRules(analysis, lbl, index, stepCount, concatNode, tokenPatternMap, action, nextID), includes
	}
	return generateRulesForNode(config, child, tokenPatternMap, nextID, lbl, action), includes
}

func handleSegmentConstructStep[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	concatNode *syntaxa.Grammar[TToken],
	index, stepCount int,
	lbl string,
	includes []StateID,
	tokenPatternMap map[TToken]Pattern,
) ([]StateRule, []StateID) {
	if index+1 < stepCount {
		if tok, ok := syntaxa.GrammarOptionalTokenChild(concatNode.Children[index+1]); ok {
			includes = append(includes, StateID(produceStateID(sanitizeContextName(config.formatter(tok)))))
		}

		nextChild := concatNode.Children[index+1]
		laAction, laNextID := getTransition(baseLabelForTransition(concatNode.GrammarID), index, index+2, stepCount)
		return buildExitRulesToNextTokenChild(config, nextChild, laAction, laNextID, lbl, tokenPatternMap), includes
	}
	return nil, includes
}

func buildLookaheadRules[TToken comparable](
	analysis *syntaxa.GrammarAnalysis[TToken],
	lbl string, index, stepCount int,
	concatNode *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
	action RuleAction, nextID StateID,
) []StateRule {
	if index+1 >= stepCount {
		return nil
	}
	suffixFirst := syntaxa.GrammarAnalysisFirstOfSuffix(analysis, concatNode, index+1)
	if la, ok := buildLookaheadForTokens(suffixFirst, tokenPatternMap); ok {
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

func getTransition(baseLabel string, currentIndex, targetIndex, stepCount int) (RuleAction, StateID) {
	if stepCount <= 1 {
		return ACTION_MATCH, 0
	}
	if currentIndex == 0 {
		if targetIndex >= stepCount {
			return ACTION_MATCH, 0
		}
		return ACTION_PUSH, StateID(produceStateID(fmt.Sprintf("%s_step_%d", baseLabel, targetIndex)))
	}
	if targetIndex >= stepCount {
		return ACTION_POP, 0
	}
	return ACTION_SET, StateID(produceStateID(fmt.Sprintf("%s_step_%d", baseLabel, targetIndex)))
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

func buildExitRulesToNextTokenChild[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nextChild *syntaxa.Grammar[TToken],
	action RuleAction,
	nextID StateID,
	stateLbl string,
	tokenPatternMap map[TToken]Pattern,
) []StateRule {
	if nextChild == nil || nextChild.Kind != syntaxa.GToken {
		return nil
	}
	return generateRulesForNode(config, nextChild, tokenPatternMap, nextID, stateLbl, action)
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
				Scope:  getScopeString(oc.Scope, config.scopeExtension),
				RegEx:  regex,
			}},
		})
	}

	allStates = append(allStates, extractConstructStates(config, plan, analysis, grammarNode, tokenPatternMap)...)
	return allStates
}

func extractConstructStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	analysis *syntaxa.GrammarAnalysis[TToken],
	root *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
) []State {
	if root == nil {
		return nil
	}

	var states []State
	_ = root.WalkPre(func(n *syntaxa.Grammar[TToken]) (skip, stop bool) {
		if n.Kind == syntaxa.GConcat {
			if _, ok := plan.ConstructConacts[n.GrammarID]; ok {
				var metaScope string
				if oc, ok2 := config.nodeOverrides[n.GrammarID]; ok2 && oc.HasMetaScope() {
					metaScope = oc.MetaScope
				}
				states = append(states, buildConstructStateChain(config, analysis, n, metaScope, tokenPatternMap)...)
			}
		}
		return false, false
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
				Scope:        getScopeString(config.scopeProvider(spec.Token), config.scopeExtension),
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

func InvalidFallbackRule(id StateRuleID, scopeExtension string) StateRule {
	return StateRule{
		ID:     id,
		Label:  invalidFallbackLabel,
		RegEx:  `\S+`,
		Scope:  getScopeString(invalidFallbackBaseScope, scopeExtension),
		Action: ACTION_MATCH,
	}
}

func getScopeString(baseScope, scopeExtension string) string {
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
			scope := config.scopeProvider(n.Token)
			if oc, ok := config.nodeOverrides[n.GrammarID]; ok && oc.HasScope() {
				scope = oc.Scope
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
		MetaScope: getScopeString(metaScopeBase, config.scopeExtension),
		Includes:  includes,
		Rules: []StateRule{
			{
				ID:     StateRuleID(produceStateID(nestLabel + "_close")),
				Label:  nestLabel + "_close",
				RegEx:  closeRegex,
				Scope:  getScopeString(config.scopeProvider(nest.Close), config.scopeExtension),
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
