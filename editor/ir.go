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
	nodeScopeOverrides   map[syntaxa.GrammarID]string
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
		nodeScopeOverrides:   make(map[syntaxa.GrammarID]string),
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

func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNodeScopeOverride(nodeID syntaxa.GrammarID, scope string) {
	c.nodeScopeOverrides[nodeID] = scope
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

	tokensInUse, tokenPatternMap, prototypeTokens := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)
	delimitedMap := extractDelimitedRules(lexingRuleSet)

	rootTokens, _, _ := extractContextIncludes(config, entryGrammar, true)

	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, prototypeTokens, rootTokens, delimitedMap)

	allStates = injectNodeOverrideStates(config, allStates, entryGrammar, tokenPatternMap)
	allStates, nestRegistry := injectNestStates(config, allStates, grammarPackage, tokenPatternMap)

	allStates = generateSequenceTriggers(allStates, config, entryGrammar, nestRegistry, tokenPatternMap)

	allStates = injectPrototypeState(allStates, prototypeIncludes)
	allStates = injectMainState(allStates, config.scopeExtension, config, entryGrammar)

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
		if errOpen != nil || errClose != nil {
			continue
		}
		out[lexerRule.Token] = DelimitedRuleRegex[TToken]{OpenRegex: openRegex, CloseRegex: closeRegex}
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
		ctx := &TokenOverrideContext{
			BaseID:          id,
			Label:           label,
			OriginalPattern: tokenPatternMap[token],
			BaseScope:       baseScope,
			ScopeExtension:  config.scopeExtension,
		}
		mainRule, extraStates = TokenOverrideDelimitedRegion(ctx,
			delimited.OpenRegex, delimited.CloseRegex,
			"punctuation.definition.comment.begin",
			baseScope,
			"punctuation.definition.comment.end",
		)
	} else if overrideFn, exists := config.overrides[token]; exists {
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
		if isUniqueToken {
			mutateStateAction(allStates, openStateID, ACTION_PUSH, entryID)
		} else {
			nestRegistry[nest.ID] = entryID
		}
		return append(allStates, customStates...)
	}

	var bodyNode *syntaxa.Grammar[TToken]
	if len(nest.Node.Children) > 0 {
		bodyNode = nest.Node.Children[0]
	}

	needsSequenceChain := bodyNode != nil && bodyNode.Kind == syntaxa.GConcat && hasNodeOverrides(config, bodyNode)

	var bodyStateID StateID
	if needsSequenceChain {
		bodyStateID = StateID(produceStateID(nestLabel + "_step_0"))
		dfaStates := buildSequenceChain(config, nest, nestLabel, tokenPatternMap, bodyStateID)
		allStates = append(allStates, dfaStates...)
	} else {
		bodyStateID = StateID(produceStateID(nestLabel + "_body"))
		bodyState := buildBodyState(config, nest, nestLabel, tokenPatternMap, bodyStateID)
		allStates = append(allStates, bodyState)
	}

	if isUniqueToken {
		mutateStateAction(allStates, openStateID, ACTION_PUSH, bodyStateID)
		return allStates
	}

	expectStateID := StateID(produceStateID(nestLabel + "_expect"))
	expectState := buildExpectState(config, nest, nestLabel, tokenPatternMap, expectStateID, bodyStateID)

	nestRegistry[nest.ID] = expectStateID
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
	validTokens, overrideIDs, triggerIDs := extractContextIncludes(config, nest.Node.Children[0], false)

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

func mutateStateAction(allStates []State, targetStateID StateID, action RuleAction, actionTarget StateID) {
	for i := range allStates {
		if allStates[i].ID == targetStateID && len(allStates[i].Rules) > 0 {
			allStates[i].Rules[0].Action = action
			allStates[i].Rules[0].ActionTarget = actionTarget
			return
		}
	}
}

// ------------------------------------------------------------------ NODE OVERRIDES

func injectNodeOverrideStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	allStates []State,
	grammarNode *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
) []State {
	visited := make(map[*syntaxa.Grammar[TToken]]bool)
	seen := make(map[syntaxa.GrammarID]bool)

	var walk func(node *syntaxa.Grammar[TToken])
	walk = func(node *syntaxa.Grammar[TToken]) {
		if node == nil || visited[node] {
			return
		}
		visited[node] = true

		if customScope, exists := config.nodeScopeOverrides[node.GrammarID]; exists && node.Kind == syntaxa.GToken {
			if !seen[node.GrammarID] {
				seen[node.GrammarID] = true

				label := fmt.Sprintf("node_override_%s", sanitizeContextName(string(node.GrammarID)))
				stateID := StateID(produceStateID(label))
				regex, _ := tokenPatternMap[node.Token].ToRegEx()

				allStates = append(allStates, State{
					ID:    stateID,
					Label: label,
					Rules: []StateRule{{
						ID:     StateRuleID(stateID),
						Label:  label + "_match",
						Action: ACTION_MATCH, // Context-local override, must match and stay in body
						Scope:  getScopeString(customScope, config.scopeExtension),
						RegEx:  regex,
					}},
					IsRootContext: false,
				})
			}
		}

		for _, child := range node.Children {
			walk(child)
		}
	}

	walk(grammarNode)
	return allStates
}

// ------------------------------------------------------------------ SEQUENCE TRIGGER GENERATION

func generateSequenceTriggers[TToken, TTokenRole comparable](
	allStates []State,
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	grammarNode *syntaxa.Grammar[TToken],
	nestRegistry map[syntaxa.GrammarID]StateID,
	tokenPatternMap map[TToken]Pattern,
) []State {
	seen := make(map[StateID]bool)
	visited := make(map[*syntaxa.Grammar[TToken]]bool)

	var walk func(node *syntaxa.Grammar[TToken])
	walk = func(node *syntaxa.Grammar[TToken]) {
		if node == nil || visited[node] {
			return
		}
		visited[node] = true

		if node.Kind == syntaxa.GConcat {
			for i := 0; i < len(node.Children)-1; i++ {
				curr, next := node.Children[i], node.Children[i+1]

				if curr.Kind == syntaxa.GToken && next.Kind == syntaxa.GNest {
					targetStateID, ok := nestRegistry[next.GrammarID]
					if !ok {
						continue
					}

					stateID := deriveTriggerID(config.formatter(curr.Token), next.GrammarID)
					if !seen[stateID] {
						seen[stateID] = true

						triggerRegex, _ := tokenPatternMap[curr.Token].ToRegEx()
						label := fmt.Sprintf("seq_trigger_%s", sanitizeContextName(string(next.GrammarID)))

						allStates = append(allStates, State{
							ID:    stateID,
							Label: label,
							Rules: []StateRule{
								{
									ID:           StateRuleID(stateID),
									Label:        label + "_match",
									Action:       ACTION_PUSH,
									ActionTarget: targetStateID,
									Scope:        getScopeString(config.scopeProvider(curr.Token), config.scopeExtension),
									RegEx:        triggerRegex,
								},
							},
							IsRootContext: false,
						})
					}
				}
			}
		}

		for _, c := range node.Children {
			walk(c)
		}
	}

	walk(grammarNode)
	return allStates
}

func deriveTriggerID(tokenLabel string, targetID syntaxa.GrammarID) StateID {
	cleanToken := sanitizeContextName(tokenLabel)
	cleanTarget := sanitizeContextName(string(targetID))
	return StateID(produceStateID(fmt.Sprintf("seq_trigger_%s_to_%s", cleanToken, cleanTarget)))
}

// ------------------------------------------------------------------ CONTEXT EXTRACTION & INJECTION

func extractContextIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	contextRoot *syntaxa.Grammar[TToken],
	isRoot bool,
) (map[TToken]struct{}, []StateID, []StateID) {
	baseTokens := make(map[TToken]struct{})
	var overrideIDs []StateID
	var triggerIDs []StateID

	visited := make(map[*syntaxa.Grammar[TToken]]bool)
	seenOverrides := make(map[StateID]bool)
	seenTriggers := make(map[StateID]bool)

	var walk func(node *syntaxa.Grammar[TToken])
	walk = func(node *syntaxa.Grammar[TToken]) {
		if node == nil || visited[node] {
			return
		}
		visited[node] = true

		switch node.Kind {
		case syntaxa.GToken:
			baseTokens[node.Token] = struct{}{}
			if _, hasOverride := config.nodeScopeOverrides[node.GrammarID]; hasOverride {
				label := fmt.Sprintf("node_override_%s", sanitizeContextName(string(node.GrammarID)))
				id := StateID(produceStateID(label))
				if !seenOverrides[id] {
					seenOverrides[id] = true
					overrideIDs = append(overrideIDs, id)
				}
			}
		case syntaxa.GNest:
			if isRoot || node != contextRoot {
				baseTokens[*node.OpenToken] = struct{}{}
				return // Do not walk inside the nest boundary.
			}
		case syntaxa.GConcat:
			for i := 0; i < len(node.Children)-1; i++ {
				curr, next := node.Children[i], node.Children[i+1]
				if curr.Kind == syntaxa.GToken && next.Kind == syntaxa.GNest {
					id := deriveTriggerID(config.formatter(curr.Token), next.GrammarID)
					if !seenTriggers[id] {
						seenTriggers[id] = true
						triggerIDs = append(triggerIDs, id)
					}
				}
			}
		}

		for _, c := range node.Children {
			walk(c)
		}
	}

	walk(contextRoot)
	return baseTokens, overrideIDs, triggerIDs
}

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

func injectMainState[TToken, TTokenRole comparable](
	allStates []State,
	scopeExtension string,
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	entryNode *syntaxa.Grammar[TToken],
) []State {
	_, overrideIDs, triggerIDs := extractContextIncludes(config, entryNode, true)

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
	if _, exists := config.nodeScopeOverrides[node.GrammarID]; exists && node.Kind == syntaxa.GToken {
		return true
	}
	for _, c := range node.Children {
		if hasNodeOverrides(config, c) {
			return true
		}
	}
	return false
}

func isNodeNullable[TToken comparable](node *syntaxa.Grammar[TToken]) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case syntaxa.GEpsilon, syntaxa.GOptional:
		return true
	case syntaxa.GRepeat:
		return node.Min == 0
	case syntaxa.GChoice:
		for _, c := range node.Children {
			if isNodeNullable(c) {
				return true
			}
		}
	case syntaxa.GConcat:
		for _, c := range node.Children {
			if !isNodeNullable(c) {
				return false
			}
		}
		return true
	}
	return false
}

func buildSequenceChain[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	entryStateID StateID,
) []State {
	concat := nest.Node.Children[0]
	var states []State
	closeRegex, _ := tokenPatternMap[nest.Close].ToRegEx()
	metaScopeBase := getScopeString(fmt.Sprintf("meta.block.%s", strings.ToLower(nestLabel)), config.scopeExtension)

	stepCount := len(concat.Children)
	for i := 0; i < stepCount; i++ {
		child := concat.Children[i]
		stateLabel := fmt.Sprintf("%s_step_%d", nestLabel, i)
		id := StateID(produceStateID(stateLabel))
		if i == 0 {
			id = entryStateID
		}

		var nextID StateID
		if i < stepCount-1 {
			nextID = StateID(produceStateID(fmt.Sprintf("%s_step_%d", nestLabel, i+1)))
		} else {
			nextID = StateID(produceStateID(nestLabel + "_tail"))
		}

		var rules []StateRule

		rules = append(rules, StateRule{
			ID:     StateRuleID(produceStateID(stateLabel + "_close")),
			Label:  stateLabel + "_close",
			RegEx:  closeRegex,
			Scope:  getScopeString(config.scopeProvider(nest.Close), config.scopeExtension),
			Action: ACTION_POP,
		})

		rules = append(rules, generateRulesForNode(config, child, tokenPatternMap, nextID, stateLabel)...)

		lookaheadIndex := i + 1
		for isNodeNullable(child) && lookaheadIndex < stepCount {
			nextChild := concat.Children[lookaheadIndex]
			var targetID StateID
			if lookaheadIndex < stepCount-1 {
				targetID = StateID(produceStateID(fmt.Sprintf("%s_step_%d", nestLabel, lookaheadIndex+1)))
			} else {
				targetID = StateID(produceStateID(nestLabel + "_tail"))
			}
			rules = append(rules, generateRulesForNode(config, nextChild, tokenPatternMap, targetID, stateLabel)...)

			if !isNodeNullable(nextChild) {
				break
			}
			lookaheadIndex++
			child = nextChild
		}

		rules = append(rules, InvalidFallbackRule(StateRuleID(produceStateID(stateLabel+"_invalid")), config.scopeExtension))

		_, _, triggerIDs := extractContextIncludes(config, child, false)

		states = append(states, State{
			ID:            id,
			Label:         stateLabel,
			MetaScope:     metaScopeBase,
			IsRootContext: false,
			Rules:         rules,
			Includes:      triggerIDs,
		})
	}

	tailLabel := nestLabel + "_tail"
	tailID := StateID(produceStateID(tailLabel))
	tailState := buildBodyState(config, nest, tailLabel, tokenPatternMap, tailID)
	tailState.Label = tailLabel
	states = append(states, tailState)

	return states
}

func generateRulesForNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	node *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
	nextID StateID,
	stateLabel string,
) []StateRule {
	var rules []StateRule

	var walk func(n *syntaxa.Grammar[TToken])
	walk = func(n *syntaxa.Grammar[TToken]) {
		if n == nil {
			return
		}
		switch n.Kind {
		case syntaxa.GToken:
			regex, _ := tokenPatternMap[n.Token].ToRegEx()
			scope := config.scopeProvider(n.Token)
			if custom, ok := config.nodeScopeOverrides[n.GrammarID]; ok {
				scope = custom
			}
			rules = append(rules, StateRule{
				ID:           StateRuleID(produceStateID(fmt.Sprintf("%s_match_%s_%v", stateLabel, n.GrammarID, n.Token))),
				Label:        fmt.Sprintf("%s_match_%s", stateLabel, n.GrammarID),
				RegEx:        regex,
				Scope:        getScopeString(scope, config.scopeExtension),
				Action:       ACTION_SET,
				ActionTarget: nextID,
			})
		case syntaxa.GChoice, syntaxa.GOptional, syntaxa.GConcat:
			for _, c := range n.Children {
				walk(c)
			}
		}
	}
	walk(node)
	return rules
}
