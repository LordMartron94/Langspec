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

/*
MustGetRegEx returns the regex string for the given token's pattern from the ruleset.
Panics if the token has no pattern or pattern.ToRegEx fails. Use when building overrides
that need the token's regex and the token is guaranteed to exist in the ruleset.
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

/*
Pattern is the RegulaAST type used for rune-based token patterns in the editor IR.
*/
type Pattern = pattern.RegulaAST[rune]

/*
StateID uniquely identifies a state in the push-down automaton IR. Opaque; use for
Includes and ActionTarget, not for semantic interpretation.
*/
type StateID uint64

/*
StateRuleID uniquely identifies a rule within the IR. Used for structural reference only.
*/
type StateRuleID uint64

//go:generate stringer -type RuleAction
/*
RuleAction is the action performed when a StateRule matches. Determines stack and
control flow (push, pop, set, match, embed) for the editor highlighter.
*/
type RuleAction uint8

const (
	ACTION_PUSH  RuleAction = iota // Push current state, enter ActionTarget state
	ACTION_POP                     // Pop state stack (optionally PopCount)
	ACTION_SET                    // Replace current state with ActionTarget (no push)
	ACTION_MATCH                  // Consume match, stay in current state
	ACTION_NONE                   // No structural action
	ACTION_EMBED                  // Embed another syntax until Escape matches
)

/*
State is one context in the push-down automaton: a label, optional meta scope, rules
that match regex and perform actions, and optional Includes (other state IDs reachable
from this context). IsRootContext marks top-level entry points; OmitPrototype excludes
the state from prototype inclusion for comment/whitespace.
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
StateRule is one rule within a State: a regex, scope to apply on match, and an action
(plus optional ActionTarget, Captures, PopCount, Embed/Escape). The highlighter tries
rules in order; first match wins.
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
}

/*
ScopeProvider maps a token to its base scope string (e.g. comment.block, string.quoted).
Supplied by the client; the IR appends scopeExtension when building final scope strings.
*/
type ScopeProvider[TToken any] func(item TToken) string

/*
TokenFormatter maps a token to a short string used for state labels and IDs. Must be
stable and safe for use in identifiers (sanitized by the IR).
*/
type TokenFormatter[TToken any] func(token TToken) string

// ------------------------------------------------------------------ OVERRIDE CONTEXTS

/*
TokenOverrideContext is the argument passed to token override functions. Carries base
state ID, label, original pattern, base scope, and scope extension so overrides can
build StateRule and extra States without duplicating ID or scope logic.
*/
type TokenOverrideContext struct {
	BaseID          StateID
	Label           string
	OriginalPattern Pattern
	BaseScope       string
	ScopeExtension  string
}

/*
DeriveStateID returns a deterministic StateID for a state derived from this context
with the given suffix (e.g. "body", "close"). Used when building multi-state overrides.
*/
func (ctx *TokenOverrideContext) DeriveStateID(suffix string) StateID {
	return StateID(produceStateID(fmt.Sprintf("%s_%s", ctx.Label, suffix)))
}

/*
ApplyScope returns scope with the context's scope extension appended. Use for all
scope strings emitted in override-built rules and states.
*/
func (ctx *TokenOverrideContext) ApplyScope(scope string) string {
	return getScopeString(scope, ctx.ScopeExtension)
}

/*
TokenOverrideFunc replaces the default single-rule state for a token with a custom
mainRule and optional extra States. Called during IR construction when the token
has an override registered.
*/
type TokenOverrideFunc func(ctx *TokenOverrideContext) (mainRule StateRule, extraStates []State)

/*
NestOverrideContext is the argument passed to nest override functions. Provides
NestLabel, scope extension, and GetRegEx(token) for building custom nest state sequences.
*/
type NestOverrideContext[TToken comparable] struct {
	NestLabel      string
	ScopeExtension string
	getRegEx       func(TToken) string
}

/*
GetRegEx returns the regex string for the given token from the context's mapping.
Used when building nest override states.
*/
func (ctx *NestOverrideContext[TToken]) GetRegEx(token TToken) string {
	return ctx.getRegEx(token)
}

/*
DeriveStateID returns a deterministic StateID for a state in this nest with the given suffix.
*/
func (ctx *NestOverrideContext[TToken]) DeriveStateID(suffix string) StateID {
	return StateID(produceStateID(fmt.Sprintf("%s_%s", ctx.NestLabel, suffix)))
}

/*
ApplyScope returns scope with the context's scope extension appended.
*/
func (ctx *NestOverrideContext[TToken]) ApplyScope(scope string) string {
	return getScopeString(scope, ctx.ScopeExtension)
}

/*
NestOverrideFunc builds a custom state sequence for a grammar nest. Returns the entry
state ID and the slice of States. Used when a nest override predicate matches.
*/
type NestOverrideFunc[TToken comparable] func(ctx *NestOverrideContext[TToken]) (entryStateID StateID, states []State)

/*
NestOverridePredicate returns true if the given nest should use a custom override
instead of the default body state. Typically matches on nest.OwnerRule.
*/
type NestOverridePredicate[TToken comparable] func(nest *syntaxa.NestSpec[TToken]) bool

// ------------------------------------------------------------------ CONFIGURATION

type nestOverrideHandler[TToken comparable] struct {
	pred NestOverridePredicate[TToken]
	fn   NestOverrideFunc[TToken]
}

/*
PushDownAutomatonIRConfiguration holds parameters and overrides for building the editor IR.
Supply scopeProvider, formatter, and scopeExtension; add prototype roles, token overrides,
and nest overrides as needed. Pass to PushDownAutomatonIRCreate with LexingRuleset and GrammarPackage.
*/
type PushDownAutomatonIRConfiguration[TToken, TTokenRole comparable] struct {
	scopeProvider        ScopeProvider[TToken]
	formatter            TokenFormatter[TToken]
	scopeExtension       string
	overrides            map[TToken]TokenOverrideFunc
	prototypeTokenRoles  []TTokenRole
	nestOverrideHandlers []nestOverrideHandler[TToken]
}

/*
PushDownAutomatonIRConfigurationCreate allocates a new configuration with the given
scope provider, token formatter, and scope extension. Add overrides and prototype roles via
AddOverride, AddNestOverride, AddPrototypeTokenRoles.
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
	}
}

/*
AddOverride registers a token override for the given token. When building base states,
this token will use the override instead of the default single-rule state. Precedence:
delimited-rule auto-generation (if any) runs first, then overrides, then default.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddOverride(token TToken, fn TokenOverrideFunc) {
	c.overrides[token] = fn
}

/*
AddNestOverride registers a nest override for the grammar rule with the given ID.
That nest will use the provided function to build custom states.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverride(ruleID syntaxa.GrammarID, fn NestOverrideFunc[TToken]) {
	c.AddNestOverrideByPredicate(func(nest *syntaxa.NestSpec[TToken]) bool {
		return nest.OwnerRule == ruleID
	}, fn)
}

/*
AddNestOverrideByPredicate registers a nest override that applies when the predicate
returns true for a nest. Use for conditional overrides (e.g. by OwnerRule).
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverrideByPredicate(pred NestOverridePredicate[TToken], fn NestOverrideFunc[TToken]) {
	c.nestOverrideHandlers = append(c.nestOverrideHandlers, nestOverrideHandler[TToken]{pred: pred, fn: fn})
}

/*
AddPrototypeTokenRoles marks token roles that should be included in the prototype state.
Tokens with these roles are reachable from the prototype context (e.g. whitespace and comments).
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddPrototypeTokenRoles(tokenRoles ...TTokenRole) {
	c.prototypeTokenRoles = append(c.prototypeTokenRoles, tokenRoles...)
}

// ------------------------------------------------------------------ IR ORCHESTRATOR

/*
PushDownAutomatonIR is the output of IR construction: language name and version,
a flat slice of States (each with rules and optional Includes), and a scope extension.
Consumed by downstream generators (e.g. Sublime syntax) to emit editor-specific format.
*/
type PushDownAutomatonIR struct {
	LanguageName    string
	LanguageVersion string
	States          []State
	ScopeExtension  string
}

/*
PushDownAutomatonIRCreate builds the full push-down automaton IR from the given
configuration, lexing ruleset, and grammar package. Extracts tokens and delimited
rules, builds base states (with automatic delimited regions and overrides), injects
nest states, wires token-to-nest transitions, then adds prototype and main states.
Returns an IR ready for serialization.

Time complexity: O(R + N + G) for ruleset size R, nests N, grammar size G.
Space complexity: O(states + rules).
*/
func PushDownAutomatonIRCreate[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TToken],
) *PushDownAutomatonIR {
	entryGrammar := grammarPackage.Rules[grammarPackage.EntryRule]
	rootTokens := extractRootWhitelistFromGrammar(entryGrammar)
	tokensInUse, tokenPatternMap, prototypeTokens := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)
	delimitedMap := extractDelimitedRules(lexingRuleSet)

	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, prototypeTokens, rootTokens, delimitedMap)

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

/*
extractLexerTokens walks the ruleset and returns tokens in use, token-to-pattern map,
and the set of tokens that have a prototype role. Used to decide which base states
to create and which to include in the prototype state.
*/
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
DelimitedRuleRegex holds precomputed open and close regex strings for a token that has
a DelimitedRule in the ruleset. The IR builder uses these to generate push/body/pop
states automatically (open PUSHes to body, body close POPs) without a token override.
*/
type DelimitedRuleRegex[TToken comparable] struct {
	OpenRegex  string
	CloseRegex string
}

/*
extractDelimitedRules builds a map from token to DelimitedRuleRegex for every token
that has a delimited rule in the ruleset. Open/close patterns are converted to regex;
tokens whose patterns fail ToRegEx are skipped.
*/
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

/*
buildBaseStates creates one State (and optional extra States) per token. For each token:
if delimited rule → open/body/close states; else if override → override; else single
ACTION_MATCH rule. Returns all states and the list of state IDs for the prototype state.
*/
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

/*
buildSingleBaseState creates the base state and optional extra states for one token.
Precedence: (1) delimited rule → TokenOverrideDelimitedRegion; (2) config override;
(3) default single-rule state. Also marks prototype include and structurally root.
*/
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

/*
injectNestStates processes each nest: applies overrides when predicate matches, else
builds default expect/body states and wires open token to PUSH to nest entry. Returns
updated states and nestRegistry (GrammarID → entry StateID) for structural wiring.
*/
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

/*
countOpenTokens returns how many nests use each token as the open token. Used to
decide whether to mutate the open state to PUSH directly (count 1) or use an expect state.
*/
func countOpenTokens[TToken comparable](nests []syntaxa.NestSpec[TToken]) map[TToken]int {
	counts := make(map[TToken]int)
	for _, nest := range nests {
		counts[nest.Open]++
	}
	return counts
}

/*
processSingleNest adds states for one nest: custom override or default expect+body.
Mutates open token state to PUSH to nest entry when open token is unique.
*/
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

/*
tryApplyNestOverride runs nest override handlers; returns custom states and entry ID
if a handler's predicate matches, else (nil, 0, false).
*/
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

/*
buildExpectState builds the "expect open" state when the open token is shared by
multiple nests: one rule matching open with ACTION_SET to body, plus invalid fallback.
*/
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

/*
buildBodyState builds the nest body state: meta scope, includes for allowed tokens
(except open/close), close rule with POP, and invalid fallback.
*/
func buildBodyState[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	nest syntaxa.NestSpec[TToken],
	nestLabel string,
	tokenPatternMap map[TToken]Pattern,
	bodyStateID StateID,
) State {
	validTokens := extractWhitelistFromNest(nest.Node)

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

/*
automateStructuralTransitions walks the grammar and wires token-to-nest transitions:
for each consecutive (token, nest) in a concat, links the token's state to PUSH to the nest entry.
*/
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

/*
wireSequence finds consecutive (GToken, GNest) pairs in a concat and links each
token's state to the nest's entry state (PUSH).
*/
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

/*
linkTokenToNest mutates the trigger token's state so its first rule does ACTION_PUSH
to the nest's entry state. No-op if nest has no entry in nestRegistry.
*/
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

/*
mutateStateAction finds the state with targetStateID and sets its first rule's
Action and ActionTarget. Used to wire token states to PUSH into nests.
*/
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

/*
injectPrototypeState appends a prototype state that Includes all prototype state IDs.
Top-level whitespace and comments are reached via this state. No-op if no includes.
*/
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

/*
injectMainState appends the root "main" state that Includes all IsRootContext states
and has an invalid fallback rule. Entry point for the highlighter.
*/
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

/*
extractWhitelistFromNest collects all tokens that can appear inside the nest's content.
Used to build the body state's Includes.
*/
func extractWhitelistFromNest[TToken comparable](nestNode *syntaxa.Grammar[TToken]) map[TToken]struct{} {
	validTokens := make(map[TToken]struct{})
	visited := make(map[*syntaxa.Grammar[TToken]]bool)

	if nestNode != nil && len(nestNode.Children) > 0 {
		collectTokensForSubtree(nestNode.Children[0], visited, validTokens)
	}
	return validTokens
}

/*
buildIncludesWhitelist returns StateIDs for tokens in validTokens excluding openTok
and closeTok. Used to set body state Includes.
*/
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

const invalidFallbackLabel     = "invalid_fallback"
const invalidFallbackBaseScope = "invalid.illegal.unexpected-token"

/*
InvalidFallbackRule returns a StateRule that matches non-whitespace and applies the
invalid scope. Used as the last rule in states that need to consume unexpected tokens.
Exported for use by override helpers.
*/
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
