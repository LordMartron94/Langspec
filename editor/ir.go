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

// ------------------------------------------------------------------ OVERRIDE CONFIG

type OverrideConfig struct {
	Scope     string
	MetaScope string
}

func (c OverrideConfig) HasScope() bool     { return c.Scope != "" }
func (c OverrideConfig) HasMetaScope() bool { return c.MetaScope != "" }
func (c OverrideConfig) HasAny() bool       { return c.HasScope() || c.HasMetaScope() }

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

	// 1) Plan pass: single source of truth
	plan := BuildIRPlan(config, entryGrammar)

	// 2) Base token states
	tokensInUse, tokenPatternMap, prototypeTokens := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)
	delimitedMap := extractDelimitedRules(lexingRuleSet)

	// Root tokens are structural-root tokens for main includes; use plan-aware extraction now.
	rootTokens, _, _ := extractContextIncludesPlanned(config, plan, entryGrammar, true)

	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, prototypeTokens, rootTokens, delimitedMap)

	// 3) Generate planned construct + override states (guarantees includes will never dangle)
	allStates = injectPlannedNodeStates(config, plan, allStates, entryGrammar, tokenPatternMap)

	// 4) Nests
	allStates, nestRegistry := injectNestStatesPlanned(config, plan, allStates, grammarPackage, tokenPatternMap)

	// 5) Triggers from the plan (no separate walking logic)
	allStates = injectPlannedSequenceTriggers(config, plan, allStates, nestRegistry, tokenPatternMap)

	// 6) Prototype + main
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

func countOpenTokens[TToken comparable](nests []syntaxa.NestSpec[TToken]) map[TToken]int {
	counts := make(map[TToken]int)
	for _, nest := range nests {
		counts[nest.Open]++
	}
	return counts
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

func nodeOverrideStateID(grammarID syntaxa.GrammarID) StateID {
	label := fmt.Sprintf("node_override_%s", sanitizeContextName(string(grammarID)))
	return StateID(produceStateID(label))
}

func nodeConstructEntryStateID(concatID syntaxa.GrammarID) StateID {
	label := fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatID)))
	return StateID(produceStateID(label))
}

func buildConstructStateChain[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	concatNode *syntaxa.Grammar[TToken],
	metaScope string,
	tokenPatternMap map[TToken]Pattern,
) []State {
	baseLabel := fmt.Sprintf("node_construct_%s", sanitizeContextName(string(concatNode.GrammarID)))
	stepCount := len(concatNode.Children)

	type stepKind uint8
	const (
		stepToken stepKind = iota
		stepSegment
	)

	isTokenChild := func(n *syntaxa.Grammar[TToken]) bool {
		return n != nil && n.Kind == syntaxa.GToken
	}

	// Token-like nullable nodes we want to “surface” without forcing a step transition.
	// (This is what fixes the optional '*' case.)
	isNullableTokenLike := func(n *syntaxa.Grammar[TToken]) (tok TToken, ok bool) {
		if n == nil {
			return tok, false
		}
		// Optional single token: GOptional -> child GToken
		if n.Kind == syntaxa.GOptional && len(n.Children) == 1 && n.Children[0] != nil && n.Children[0].Kind == syntaxa.GToken {
			return n.Children[0].Token, true
		}
		return tok, false
	}

	getTransition := func(i, targetIndex int) (RuleAction, StateID) {
		if stepCount == 1 {
			return ACTION_MATCH, 0
		}
		if i == 0 {
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

	stepLabel := func(i int) string {
		if stepCount <= 1 {
			return baseLabel
		}
		return fmt.Sprintf("%s_step_%d", baseLabel, i)
	}

	stepID := func(i int) StateID {
		if i == 0 {
			return nodeConstructEntryStateID(concatNode.GrammarID)
		}
		return StateID(produceStateID(stepLabel(i)))
	}

	// FIRST-set approximation over grammar structure (token-only).
	// - For Nest: FIRST is its open token
	// - For Optional/Repeat(min=0): include FIRST(child)
	// - For Choice: union FIRST(children)
	// - For Concat: FIRST(first child), and if child nullable, include FIRST(next), etc.
	firstTokens := func(n *syntaxa.Grammar[TToken]) map[TToken]struct{} {
		out := make(map[TToken]struct{})
		var rec func(x *syntaxa.Grammar[TToken])
		rec = func(x *syntaxa.Grammar[TToken]) {
			if x == nil {
				return
			}
			switch x.Kind {
			case syntaxa.GToken:
				out[x.Token] = struct{}{}
			case syntaxa.GNest:
				if x.OpenToken != nil {
					out[*x.OpenToken] = struct{}{}
				}
			case syntaxa.GChoice:
				for _, c := range x.Children {
					rec(c)
				}
			case syntaxa.GOptional:
				if len(x.Children) > 0 {
					rec(x.Children[0])
				}
			case syntaxa.GRepeat:
				if len(x.Children) > 0 {
					rec(x.Children[0])
				}
			case syntaxa.GConcat:
				for _, c := range x.Children {
					rec(c)
					if !isNodeNullable(c) {
						break
					}
				}
			case syntaxa.GEpsilon:
				// none
			default:
				for _, c := range x.Children {
					rec(c)
				}
			}
		}
		rec(n)
		return out
	}

	// FIRST of suffix children[start:], skipping over nullable nodes.
	// This is the fix for Optional(...) followed by a required token (e.g. Optional(META) then ';').
	firstTokensOfSuffix := func(start int) map[TToken]struct{} {
		out := make(map[TToken]struct{})
		for j := start; j < stepCount; j++ {
			ft := firstTokens(concatNode.Children[j])
			for t := range ft {
				out[t] = struct{}{}
			}
			if !isNodeNullable(concatNode.Children[j]) {
				break
			}
		}
		return out
	}

	// Build a zero-width lookahead regex for “next tokens”, allowing whitespace in between.
	buildLookaheadForTokens := func(toks map[TToken]struct{}) (string, bool) {
		if len(toks) == 0 {
			return "", false
		}
		parts := make([]string, 0, len(toks))
		for t := range toks {
			r, err := tokenPatternMap[t].ToRegEx()
			if err != nil || r == "" {
				continue
			}
			parts = append(parts, fmt.Sprintf("(?:%s)", r))
		}
		if len(parts) == 0 {
			return "", false
		}
		return fmt.Sprintf(`(?=\s*(?:%s))`, strings.Join(parts, "|")), true
	}

	buildIncludesForNode := func(n *syntaxa.Grammar[TToken]) []StateID {
		validTokens, overrideIDs, triggerIDs := extractContextIncludes(config, n, false)

		// Structural: if the root is a Nest, allow its open token to start the nest.
		if n != nil && n.Kind == syntaxa.GNest && n.OpenToken != nil {
			validTokens[*n.OpenToken] = struct{}{}
		}

		var includes []StateID
		includes = append(includes, triggerIDs...)
		includes = append(includes, overrideIDs...)
		includes = append(includes, buildIncludesWhitelist(validTokens, *new(TToken), *new(TToken), func(tok TToken) StateID {
			return StateID(produceStateID(sanitizeContextName(config.formatter(tok))))
		})...)
		return includes
	}

	// Exit only via immediate next token child (when it exists)
	buildExitRulesToNextTokenChild := func(
		nextChild *syntaxa.Grammar[TToken],
		action RuleAction,
		nextID StateID,
		stateLbl string,
	) []StateRule {
		if nextChild == nil || nextChild.Kind != syntaxa.GToken {
			return nil
		}
		return generateRulesForNode(config, nextChild, tokenPatternMap, nextID, stateLbl, action)
	}

	var states []State

	for i := 0; i < stepCount; i++ {
		child := concatNode.Children[i]
		lbl := stepLabel(i)
		id := stepID(i)

		kind := stepSegment
		if isTokenChild(child) {
			kind = stepToken
		}

		var rules []StateRule
		var includes []StateID

		switch kind {
		case stepToken:
			action, nextID := getTransition(i, i+1)

			// If this token has a token-level override (embed etc.), we can't emit a direct match rule.
			// Instead: keep it as include, and add a SPECIFIC lookahead transition to the next step.
			if child != nil && child.Kind == syntaxa.GToken {
				if _, hasTokOverride := config.overrides[child.Token]; hasTokOverride {
					includes = buildIncludesForNode(child)

					// FIX: lookahead uses FIRST of the whole suffix, skipping nullable nodes.
					if i+1 < stepCount {
						suffixFirst := firstTokensOfSuffix(i + 1)
						if la, ok := buildLookaheadForTokens(suffixFirst); ok {
							rules = append(rules, StateRule{
								ID:           StateRuleID(produceStateID(lbl + "_advance_la")),
								Label:        lbl + "_advance_la",
								RegEx:        la,
								Action:       action,
								ActionTarget: nextID,
							})
						}
					}
					break
				}
			}

			// Normal token step: direct match then advance
			rules = append(rules, generateRulesForNode(config, child, tokenPatternMap, nextID, lbl, action)...)
			includes = buildIncludesForNode(child)

		case stepSegment:
			includes = buildIncludesForNode(child)

			// Also surface FIRST tokens of immediately following nullable token-like siblings
			// so operators like '*' get scoped without needing a state transition.
			if i+1 < stepCount {
				if tok, ok := isNullableTokenLike(concatNode.Children[i+1]); ok {
					includes = append(includes, StateID(produceStateID(sanitizeContextName(config.formatter(tok)))))
				}
			}

			// Exit only via immediate next token child
			if i+1 < stepCount {
				nextChild := concatNode.Children[i+1]
				laAction, laNextID := getTransition(i, i+2)
				rules = append(rules, buildExitRulesToNextTokenChild(nextChild, laAction, laNextID, lbl)...)
			}
		}

		st := State{
			ID:       id,
			Label:    lbl,
			Rules:    rules,
			Includes: includes,
		}
		if i > 0 && metaScope != "" {
			st.MetaScope = getScopeString(metaScope, config.scopeExtension)
		}
		states = append(states, st)
	}

	return states
}

func injectPlannedNodeStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	allStates []State,
	grammarNode *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
) []State {
	// Emit all node_override_* states from the plan
	for gid := range plan.OverrideTokens {
		// We need the token for this grammar ID; traverse to find it.
		// (If you want faster, store token in the plan too.)
		var tok *syntaxa.Grammar[TToken]
		var find func(n *syntaxa.Grammar[TToken])
		find = func(n *syntaxa.Grammar[TToken]) {
			if n == nil || tok != nil {
				return
			}
			if n.GrammarID == gid && n.Kind == syntaxa.GToken {
				tok = n
				return
			}
			for _, c := range n.Children {
				find(c)
			}
		}
		find(grammarNode)
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

	// Emit all node_construct_* chains from the plan
	visited := make(map[*syntaxa.Grammar[TToken]]bool)
	var walk func(n *syntaxa.Grammar[TToken])
	walk = func(n *syntaxa.Grammar[TToken]) {
		if n == nil || visited[n] {
			return
		}
		visited[n] = true

		if n.Kind == syntaxa.GConcat {
			if _, ok := plan.ConstructConacts[n.GrammarID]; ok {
				var metaScope string
				if oc, ok2 := config.nodeOverrides[n.GrammarID]; ok2 && oc.HasMetaScope() {
					metaScope = oc.MetaScope
				}
				allStates = append(allStates, buildConstructStateChain(config, n, metaScope, tokenPatternMap)...)
				// Do not return; keep walking for other constructs.
			}
		}

		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(grammarNode)

	return allStates
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
	OverrideIDs  []StateID // node_override_*
	ConstructIDs []StateID // node_construct_* (entry IDs)
}

func analyzeIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	root *syntaxa.Grammar[TToken],
	isRoot bool,
) includeAnalysis[TToken] {
	out := includeAnalysis[TToken]{
		ValidTokens:  make(map[TToken]struct{}),
		TriggerIDs:   nil,
		OverrideIDs:  nil,
		ConstructIDs: nil,
	}

	visited := make(map[*syntaxa.Grammar[TToken]]bool)
	seenTriggers := make(map[StateID]bool)
	seenOverrides := make(map[StateID]bool)
	seenConstructs := make(map[StateID]bool)

	// If root is a Nest, allow it to start.
	if root != nil && root.Kind == syntaxa.GNest && root.OpenToken != nil {
		out.ValidTokens[*root.OpenToken] = struct{}{}
	}

	var walk func(n *syntaxa.Grammar[TToken])
	walk = func(n *syntaxa.Grammar[TToken]) {
		if n == nil || visited[n] {
			return
		}
		visited[n] = true

		// If this concat contains overrides, include the construct entry and STOP descending
		// into it for token collection (otherwise you lose the “position-aware” behavior).
		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			cid := nodeConstructEntryStateID(n.GrammarID)
			if !seenConstructs[cid] {
				seenConstructs[cid] = true
				out.ConstructIDs = append(out.ConstructIDs, cid)
			}
			return
		}

		// Concat triggers: token followed by nest
		if n.Kind == syntaxa.GConcat {
			for i := 0; i < len(n.Children)-1; i++ {
				curr, next := n.Children[i], n.Children[i+1]
				if curr.Kind == syntaxa.GToken && next.Kind == syntaxa.GNest {
					id := deriveTriggerID(config.formatter(curr.Token), next.GrammarID)
					if !seenTriggers[id] {
						seenTriggers[id] = true
						out.TriggerIDs = append(out.TriggerIDs, id)
					}
				}
			}
		}

		switch n.Kind {
		case syntaxa.GToken:
			// If there is a node scope override, include node_override_* as a position-aware alternative.
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
			// Opaque boundary
			if isRoot || n != root {
				if n.OpenToken != nil {
					out.ValidTokens[*n.OpenToken] = struct{}{}
				}
				return
			}
		}

		for _, c := range n.Children {
			walk(c)
		}
	}

	walk(root)
	return out
}

func extractContextIncludes[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	contextRoot *syntaxa.Grammar[TToken],
	isRoot bool,
) (map[TToken]struct{}, []StateID, []StateID) {
	a := analyzeIncludes(config, contextRoot, isRoot)

	// IMPORTANT: construct IDs must be included before token IDs, otherwise you fall back to dumb token-only highlighting.
	// override IDs should come before token IDs as well, for the same reason.
	var overrides []StateID
	overrides = append(overrides, a.ConstructIDs...)
	overrides = append(overrides, a.OverrideIDs...)

	return a.ValidTokens, overrides, a.TriggerIDs
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

	if oc, exists := config.nodeOverrides[node.GrammarID]; exists && oc.HasAny() {
		if (node.Kind == syntaxa.GToken && oc.HasScope()) || (node.Kind == syntaxa.GConcat && oc.HasMetaScope()) {
			return true
		}
	}

	// CRITICAL: Nests are opaque structural boundaries.
	if node.Kind == syntaxa.GNest {
		return false
	}

	for _, c := range node.Children {
		if hasNodeOverrides(config, c) {
			return true
		}
	}

	return false
}

func generateRulesForNode[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	node *syntaxa.Grammar[TToken],
	tokenPatternMap map[TToken]Pattern,
	nextID StateID,
	stateLabel string,
	action RuleAction,
) []StateRule {
	var rules []StateRule

	var walk func(n *syntaxa.Grammar[TToken])
	walk = func(n *syntaxa.Grammar[TToken]) {
		if n == nil {
			return
		}
		switch n.Kind {
		case syntaxa.GToken:
			if _, hasTokOverride := config.overrides[n.Token]; hasTokOverride {
				return
			}

			regex, _ := tokenPatternMap[n.Token].ToRegEx()
			scope := config.scopeProvider(n.Token)
			if oc, ok := config.nodeOverrides[n.GrammarID]; ok && oc.HasScope() {
				scope = oc.Scope
			}

			rules = append(rules, StateRule{
				ID:           StateRuleID(produceStateID(fmt.Sprintf("%s_match_%s_%v", stateLabel, n.GrammarID, n.Token))),
				Label:        fmt.Sprintf("%s_match_%s", stateLabel, n.GrammarID),
				RegEx:        regex,
				Scope:        getScopeString(scope, config.scopeExtension),
				Action:       action,
				ActionTarget: nextID,
			})

		case syntaxa.GChoice, syntaxa.GOptional, syntaxa.GConcat, syntaxa.GRepeat:
			for _, c := range n.Children {
				walk(c)
			}
		}
	}
	walk(node)
	return rules
}

type IRPlan[TToken comparable] struct {
	// Concat nodes that must be represented by a node_construct_* chain.
	ConstructConacts map[syntaxa.GrammarID]struct{}

	// Token nodes that must be represented by a node_override_* state.
	OverrideTokens map[syntaxa.GrammarID]struct{}

	// All sequence triggers required by token->nest adjacency.
	SeqTriggers map[StateID]seqTriggerSpec[TToken]
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

	visited := make(map[*syntaxa.Grammar[TToken]]bool)

	var walk func(n *syntaxa.Grammar[TToken])
	walk = func(n *syntaxa.Grammar[TToken]) {
		if n == nil || visited[n] {
			return
		}
		visited[n] = true

		// Mark constructs (concat nodes) that require a DFA chain.
		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			plan.ConstructConacts[n.GrammarID] = struct{}{}
		}

		// Mark node override tokens.
		if n.Kind == syntaxa.GToken {
			if oc, ok := config.nodeOverrides[n.GrammarID]; ok && oc.HasScope() {
				plan.OverrideTokens[n.GrammarID] = struct{}{}
			}
		}

		// Collect token->nest triggers.
		if n.Kind == syntaxa.GConcat {
			for i := 0; i < len(n.Children)-1; i++ {
				curr, next := n.Children[i], n.Children[i+1]
				if curr != nil && next != nil && curr.Kind == syntaxa.GToken && next.Kind == syntaxa.GNest {
					id := deriveTriggerID(config.formatter(curr.Token), next.GrammarID)
					if _, exists := plan.SeqTriggers[id]; !exists {
						plan.SeqTriggers[id] = seqTriggerSpec[TToken]{
							ID:         id,
							Label:      fmt.Sprintf("seq_trigger_%s", sanitizeContextName(string(next.GrammarID))),
							Token:      curr.Token,
							TargetNest: next.GrammarID,
						}
					}
				}
			}
		}

		// IMPORTANT: plan must traverse into nest bodies too.
		for _, c := range n.Children {
			walk(c)
		}
	}

	walk(root)
	return plan
}

func extractContextIncludesPlanned[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	contextRoot *syntaxa.Grammar[TToken],
	isRoot bool,
) (map[TToken]struct{}, []StateID, []StateID) {
	// Use your existing analyzeIncludes logic, but when it encounters an override concat,
	// include construct IDs ONLY if that concat is in the plan.
	a := analyzeIncludesWithPlan(config, plan, contextRoot, isRoot)

	var overrides []StateID
	overrides = append(overrides, a.ConstructIDs...)
	overrides = append(overrides, a.OverrideIDs...)
	return a.ValidTokens, overrides, a.TriggerIDs
}

func analyzeIncludesWithPlan[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	root *syntaxa.Grammar[TToken],
	isRoot bool,
) includeAnalysis[TToken] {
	out := includeAnalysis[TToken]{
		ValidTokens:  make(map[TToken]struct{}),
		TriggerIDs:   nil,
		OverrideIDs:  nil,
		ConstructIDs: nil,
	}

	visited := make(map[*syntaxa.Grammar[TToken]]bool)
	seenTriggers := make(map[StateID]bool)
	seenOverrides := make(map[StateID]bool)
	seenConstructs := make(map[StateID]bool)

	if root != nil && root.Kind == syntaxa.GNest && root.OpenToken != nil {
		out.ValidTokens[*root.OpenToken] = struct{}{}
	}

	var walk func(n *syntaxa.Grammar[TToken])
	walk = func(n *syntaxa.Grammar[TToken]) {
		if n == nil || visited[n] {
			return
		}
		visited[n] = true

		if n.Kind == syntaxa.GConcat && hasNodeOverrides(config, n) {
			// Only include if planned (guarantees the chain exists).
			if _, ok := plan.ConstructConacts[n.GrammarID]; ok {
				cid := nodeConstructEntryStateID(n.GrammarID)
				if !seenConstructs[cid] {
					seenConstructs[cid] = true
					out.ConstructIDs = append(out.ConstructIDs, cid)
				}
				return
			}
			// If not planned, fall through to token collection.
		}

		if n.Kind == syntaxa.GConcat {
			for i := 0; i < len(n.Children)-1; i++ {
				curr, next := n.Children[i], n.Children[i+1]
				if curr.Kind == syntaxa.GToken && next.Kind == syntaxa.GNest {
					id := deriveTriggerID(config.formatter(curr.Token), next.GrammarID)
					if !seenTriggers[id] {
						seenTriggers[id] = true
						out.TriggerIDs = append(out.TriggerIDs, id)
					}
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
				return
			}
		}

		for _, c := range n.Children {
			walk(c)
		}
	}

	walk(root)
	return out
}

func buildBodyStatePlanned[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	plan *IRPlan[TToken],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	bodyStateID StateID,
) State {
	validTokens, overrideIDs, triggerIDs := extractContextIncludesPlanned(config, plan, nest.Node.Children[0], false)

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
	openTokenCounts := countOpenTokens(grammarPackage.Nests)
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
	_, overrideIDs, triggerIDs := extractContextIncludesPlanned(config, plan, entryNode, true)

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

func isNodeNullable[TToken comparable](node *syntaxa.Grammar[TToken]) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case syntaxa.GEpsilon, syntaxa.GOptional, syntaxa.GNest:
		// - epsilon is nullable by definition
		// - optional has an ε path
		// - nest nodes are treated as nullable for static lookahead because they do not
		//   necessarily consume a token at the current position (the open token is handled
		//   via triggers / includes at boundaries)
		return true

	case syntaxa.GRepeat:
		// 0..∞ or 0..N is nullable
		return node.Min == 0

	case syntaxa.GChoice:
		// nullable if any alternative is nullable
		for _, c := range node.Children {
			if isNodeNullable(c) {
				return true
			}
		}
		return false

	case syntaxa.GConcat:
		// nullable if all children are nullable
		for _, c := range node.Children {
			if !isNodeNullable(c) {
				return false
			}
		}
		return true

	default:
		// token and everything else: non-nullable by default
		return false
	}
}
