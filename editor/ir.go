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

// ------------------------------------------------------------------ TYPES

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
	nestExtraIncludes    map[syntaxa.GrammarID][]TToken
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
		nestExtraIncludes:    make(map[syntaxa.GrammarID][]TToken),
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

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddExtraNestIncludes(ruleID syntaxa.GrammarID, tokens ...TToken) {
	c.nestExtraIncludes[ruleID] = append(c.nestExtraIncludes[ruleID], tokens...)
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
	rootTokens := extractRootWhitelistFromGrammar(entryGrammar)
	tokensInUse, tokenPatternMap, prototypeTokens := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)

	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, prototypeTokens, rootTokens)

	allStates, nestRegistry := injectNestStates(config, allStates, grammarPackage, tokenPatternMap)
	automateStructuralTransitions(allStates, entryGrammar, config, nestRegistry)

	allStates = injectPrototypeState(allStates, prototypeIncludes)
	allStates = injectMainState(allStates, config.scopeExtension)

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

func buildBaseStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	tokensInUse []TToken,
	tokenPatternMap map[TToken]Pattern,
	prototypeTokens map[TToken]bool,
	rootTokens map[TToken]struct{},
) ([]State, []StateID) {
	var allStates []State
	var prototypeIncludes []StateID

	for _, token := range tokensInUse {
		state, extraStates, isProto := buildSingleBaseState(config, token, tokenPatternMap, prototypeTokens, rootTokens)

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
) (State, []State, bool) {
	label := sanitizeContextName(config.formatter(token))
	id := StateID(produceStateID(label))
	baseScope := config.scopeProvider(token)

	var mainRule StateRule
	var extraStates []State

	if overrideFn, exists := config.overrides[token]; exists {
		ctx := &TokenOverrideContext{
			BaseID:          id,
			Label:           label,
			OriginalPattern: tokenPatternMap[token],
			BaseScope:       baseScope,
			ScopeExtension:  config.scopeExtension,
		}
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

// ------------------------------------------------------------------ NEST COMPILER

func injectNestStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	allStates []State,
	grammarPackage syntaxa.GrammarPackage[TToken],
	tokenPatternMap map[TToken]Pattern,
) ([]State, map[syntaxa.GrammarID]StateID) {

	openTokenCounts := countOpenTokens(grammarPackage.Nests)
	nestRegistry := make(map[syntaxa.GrammarID]StateID)

	for _, nest := range grammarPackage.Nests {
		allStates = processSingleNest(config, allStates, nest, tokenPatternMap, openTokenCounts, nestRegistry)
	}

	return allStates, nestRegistry
}

func countOpenTokens[TToken comparable](nests []syntaxa.NestSpec[TToken]) map[TToken]int {
	counts := make(map[TToken]int)
	for _, nest := range nests {
		counts[nest.Open]++
	}
	return counts
}

func processSingleNest[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	allStates []State,
	nest syntaxa.NestSpec[TToken],
	tokenPatternMap map[TToken]Pattern,
	openTokenCounts map[TToken]int,
	nestRegistry map[syntaxa.GrammarID]StateID,
) []State {
	nestLabel := sanitizeContextName(string(nest.OwnerRule))

	isUniqueToken := openTokenCounts[nest.Open] == 1
	openStateID := StateID(produceStateID(sanitizeContextName(config.formatter(nest.Open))))

	if customStates, entryID, handled := tryApplyNestOverride(config, nest, nestLabel, tokenPatternMap); handled {
		nestRegistry[nest.OwnerRule] = entryID

		if isUniqueToken {
			mutateStateAction(allStates, openStateID, ACTION_PUSH, entryID)
		}

		return append(allStates, customStates...)
	}

	bodyStateID := StateID(produceStateID(nestLabel + "_body"))
	bodyState := buildBodyState(config, nest, nestLabel, tokenPatternMap, bodyStateID)
	allStates = append(allStates, bodyState)

	if isUniqueToken {
		mutateStateAction(allStates, openStateID, ACTION_PUSH, bodyStateID)
		return allStates
	}

	expectStateID := StateID(produceStateID(nestLabel + "_expect"))
	expectState := buildExpectState(config, nest, nestLabel, tokenPatternMap, expectStateID, bodyStateID)

	nestRegistry[nest.OwnerRule] = expectStateID
	return append(allStates, expectState)
}

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

func buildBodyState[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	bodyStateID StateID,
) State {
	validTokens := extractWhitelistFromNest(nest.Node)

	for _, extraTok := range config.nestExtraIncludes[nest.OwnerRule] {
		validTokens[extraTok] = struct{}{}
	}

	includes := buildIncludesWhitelist(validTokens, nest.Open, nest.Close, func(tok TToken) StateID {
		return StateID(produceStateID(sanitizeContextName(config.formatter(tok))))
	})

	closeRegex, _ := tokenPatternMap[nest.Close].ToRegEx()
	metaScopeBase := fmt.Sprintf("meta.block.%s", strings.ToLower(nestLabel))

	return State{
		ID:            bodyStateID,
		Label:         nestLabel + "_body",
		IsRootContext: false,
		MetaScope:     getScopeString(metaScopeBase, config.scopeExtension),
		Includes:      includes,
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

// ------------------------------------------------------------------ STRUCTURAL WIRING

func automateStructuralTransitions[TToken, TTokenRole comparable](
	allStates []State,
	grammarNode *syntaxa.Grammar[TToken],
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nestRegistry map[syntaxa.GrammarID]StateID,
) {
	if grammarNode == nil {
		return
	}

	if grammarNode.Kind == syntaxa.GConcat {
		wireSequence(allStates, grammarNode, config, nestRegistry)
	}

	for _, child := range grammarNode.Children {
		automateStructuralTransitions(allStates, child, config, nestRegistry)
	}
}

func wireSequence[TToken, TTokenRole comparable](
	allStates []State,
	concatNode *syntaxa.Grammar[TToken],
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nestRegistry map[syntaxa.GrammarID]StateID,
) {
	children := concatNode.Children
	if len(children) < 2 {
		return
	}

	for i := 0; i < len(children)-1; i++ {
		current := children[i]
		next := children[i+1]

		if current.Kind == syntaxa.GToken && next.Kind == syntaxa.GNest {
			linkTokenToNest(allStates, current.Token, next, config, nestRegistry)
		}
	}
}

func linkTokenToNest[TToken, TTokenRole comparable](
	allStates []State,
	triggerToken TToken,
	nestNode *syntaxa.Grammar[TToken],
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nestRegistry map[syntaxa.GrammarID]StateID,
) {
	entryStateID, ok := nestRegistry[nestNode.GrammarID]
	if !ok {
		return
	}

	triggerLabel := sanitizeContextName(config.formatter(triggerToken))
	triggerStateID := StateID(produceStateID(triggerLabel))

	mutateStateAction(allStates, triggerStateID, ACTION_PUSH, entryStateID)
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

// ------------------------------------------------------------------ INJECTIONS & UTILS

func injectPrototypeState(allStates []State, prototypeIncludes []StateID) []State {
	if len(prototypeIncludes) == 0 {
		return allStates
	}

	protoState := State{
		ID:            StateID(produceStateID("prototype")),
		Label:         "prototype",
		IsRootContext: false,
		Includes:      prototypeIncludes,
	}

	return append(allStates, protoState)
}

func injectMainState(allStates []State, scopeExtension string) []State {
	var rootIncludes []StateID
	for _, st := range allStates {
		if st.IsRootContext {
			rootIncludes = append(rootIncludes, st.ID)
		}
	}
	mainState := State{
		ID:            StateID(produceStateID("main")),
		Label:         "main",
		IsRootContext: false,
		Includes:      rootIncludes,
		Rules: []StateRule{
			InvalidFallbackRule(StateRuleID(produceStateID("main_invalid")), scopeExtension),
		},
	}
	return append(allStates, mainState)
}

func extractWhitelistFromNest[TToken comparable](nestNode *syntaxa.Grammar[TToken]) map[TToken]struct{} {
	validTokens := make(map[TToken]struct{})
	visited := make(map[*syntaxa.Grammar[TToken]]bool)

	if nestNode != nil && len(nestNode.Children) > 0 {
		collectTokensForSubtree(nestNode.Children[0], visited, validTokens)
	}
	return validTokens
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

const invalidFallbackLabel = "invalid_fallback"
const invalidFallbackBaseScope = "invalid.illegal.unexpected-token"

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

func collectTokensForSubtree[TToken comparable](
	g *syntaxa.Grammar[TToken],
	visited map[*syntaxa.Grammar[TToken]]bool,
	out map[TToken]struct{},
) {
	if g == nil || visited[g] {
		return
	}
	visited[g] = true

	switch g.Kind {
	case syntaxa.GToken:
		out[g.Token] = struct{}{}
	case syntaxa.GNest:
		out[*g.OpenToken] = struct{}{}
		out[*g.CloseToken] = struct{}{}
		for _, child := range g.Children {
			collectTokensForSubtree(child, visited, out)
		}
	case syntaxa.GConcat, syntaxa.GChoice, syntaxa.GRepeat, syntaxa.GOptional:
		for _, child := range g.Children {
			collectTokensForSubtree(child, visited, out)
		}
	}
}

func extractRootWhitelistFromGrammar[TToken comparable](g *syntaxa.Grammar[TToken]) map[TToken]struct{} {
	validTokens := make(map[TToken]struct{})
	visited := make(map[*syntaxa.Grammar[TToken]]bool)

	var walk func(node *syntaxa.Grammar[TToken])
	walk = func(node *syntaxa.Grammar[TToken]) {
		if node == nil || visited[node] {
			return
		}
		visited[node] = true

		switch node.Kind {
		case syntaxa.GToken:
			validTokens[node.Token] = struct{}{}
		case syntaxa.GNest:
			validTokens[*node.OpenToken] = struct{}{}
		case syntaxa.GConcat, syntaxa.GChoice, syntaxa.GRepeat, syntaxa.GOptional:
			for _, child := range node.Children {
				walk(child)
			}
		}
	}

	walk(g)
	return validTokens
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
