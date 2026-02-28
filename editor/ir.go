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

/*
Pattern is the observation pattern type used for token recognition in the editor IR.
It is an alias for the Regula AST from autarch/pattern.
*/
type Pattern = pattern.RegulaAST[rune]

/*
StateID is a unique identifier for a state.
*/
type StateID uint64

/*
StateRuleID is a unique identifier for a state rule.
*/
type StateRuleID uint64

/*
RuleAction represents an action to do for a rule.
Only one of PUSH, POP, SET, EMBED may be used per rule (per ST4 syntax).
*/
//go:generate stringer -type RuleAction
type RuleAction uint8

const (
	ACTION_PUSH RuleAction = iota
	ACTION_POP
	ACTION_SET
	ACTION_MATCH
	ACTION_NONE
	ACTION_EMBED // Embed another syntax; use Embed, EmbedScope, Escape, EscapeCaptures. Mutually exclusive with PUSH/SET/POP.
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
	ID              StateRuleID
	Label           string
	RegEx           string
	Scope           string
	Action          RuleAction
	ActionTarget    StateID
	Captures        map[int]string
	PopCount        int
	// Embed fields: only used when Action == ACTION_EMBED (ST4 embed/escape).
	Embed           string            // Target context to embed, e.g. scope:source.regexp
	EmbedScope      string            // Scope applied to the embedded region
	Escape         string            // Regex that ends the embedded region (required when using embed)
	EscapeCaptures  map[int]string    // Scopes for capture groups of the escape pattern; 0 = entire escape match
}

type ScopeProvider[T any] func(item T) string

type TokenFormatter[TToken any] func(token TToken) string

/*
TokenOverrideContext provides the necessary utilities and state to a TokenOverrideFunc,
allowing clients to generate rules and states without breaking package boundaries.
*/
type TokenOverrideContext struct {
	BaseID          StateID
	Label           string
	OriginalPattern Pattern
	BaseScope       string
	ScopeExtension  string
}

/*
DeriveStateID generates a deterministic ID for auxiliary states using the editor package's internal hasher.
*/
func (ctx *TokenOverrideContext) DeriveStateID(suffix string) StateID {
	return StateID(produceStateID(fmt.Sprintf("%s_%s", ctx.Label, suffix)))
}

/*
ApplyScope appends the configured scope extension to a base scope string.
*/
func (ctx *TokenOverrideContext) ApplyScope(scope string) string {
	return getScopeString(scope, ctx.ScopeExtension)
}

/*
TokenOverrideFunc generates a custom rule and optional auxiliary states for a token.
*/
type TokenOverrideFunc func(ctx *TokenOverrideContext) (mainRule StateRule, extraStates []State)

/*
NestOverrideContext provides utilities and state to a NestOverrideFunc. GetRegEx resolves
a token to its regex pattern using the grammar's token map; the client need not close over
a ruleset.
*/
type NestOverrideContext[TToken comparable] struct {
	NestLabel      string
	ScopeExtension string
	getRegEx      func(TToken) string
}

/*
GetRegEx returns the regex string for the given token. Used when building custom nest
states so the client does not need access to the lexing ruleset.
*/
func (ctx *NestOverrideContext[TToken]) GetRegEx(token TToken) string {
	return ctx.getRegEx(token)
}

/*
DeriveStateID generates a deterministic ID for auxiliary states using the editor package's internal hasher.
*/
func (ctx *NestOverrideContext[TToken]) DeriveStateID(suffix string) StateID {
	return StateID(produceStateID(fmt.Sprintf("%s_%s", ctx.NestLabel, suffix)))
}

/*
ApplyScope appends the configured scope extension to a base scope string.
*/
func (ctx *NestOverrideContext[TToken]) ApplyScope(scope string) string {
	return getScopeString(scope, ctx.ScopeExtension)
}

/*
NestOverrideFunc generates a custom entry state ID and states for a grammar nest.
*/
type NestOverrideFunc[TToken comparable] func(ctx *NestOverrideContext[TToken]) (entryStateID StateID, states []State)

/*
NestOverridePredicate returns true if the given nest should use the associated override.
The client can match by nest.OwnerRule, nest.Open/nest.Close, or any custom logic.
*/
type NestOverridePredicate[TToken comparable] func(nest *syntaxa.NestSpec[TToken]) bool

// ------------------------------------------------------------------ CONFIGURATION

/*
PushDownAutomatonIRConfiguration holds scope provider, token formatter, scope extension,
token overrides, nest overrides, and prototype token roles. Create with
PushDownAutomatonIRConfigurationCreate, then add overrides and prototype roles before
calling PushDownAutomatonIRCreate.
*/
type nestOverrideHandler[TToken comparable] struct {
	pred NestOverridePredicate[TToken]
	fn   NestOverrideFunc[TToken]
}

type PushDownAutomatonIRConfiguration[TToken, TTokenRole comparable] struct {
	scopeProvider         ScopeProvider[TToken]
	formatter             TokenFormatter[TToken]
	scopeExtension        string
	overrides             map[TToken]TokenOverrideFunc
	prototypeTokenRoles   []TTokenRole
	nestOverrideHandlers  []nestOverrideHandler[TToken]
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
		nestOverrideHandlers: nil,
	}
}

/*
AddOverride registers a custom rule and optional extra states for a token. When building
the IR, this function is invoked with a TokenOverrideContext; use the structural helpers
TokenOverrideDelimitedRegion or TokenOverrideMatchWithCapture, or build StateRule/State by hand.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddOverride(token TToken, fn TokenOverrideFunc) {
	c.overrides[token] = fn
}

/*
AddNestOverride registers a custom state sequence for a nest whose OwnerRule equals
ruleID. It is a convenience over AddNestOverrideByPredicate that registers a predicate
nest.OwnerRule == ruleID.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverride(ruleID syntaxa.GrammarID, fn NestOverrideFunc[TToken]) {
	c.AddNestOverrideByPredicate(func(nest *syntaxa.NestSpec[TToken]) bool {
		return nest.OwnerRule == ruleID
	}, fn)
}

/*
AddNestOverrideByPredicate registers a custom state sequence for any nest that matches
pred. The first registered predicate that returns true for a nest wins. The callback
receives a NestOverrideContext with GetRegEx for token lookup; use BuildNestStateSequence
with declarative NestStep slices to build states without manual construction.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddNestOverrideByPredicate(pred NestOverridePredicate[TToken], fn NestOverrideFunc[TToken]) {
	c.nestOverrideHandlers = append(c.nestOverrideHandlers, nestOverrideHandler[TToken]{pred: pred, fn: fn})
}

/*
AddPrototypeTokenRoles registers a token (e.g. whitespace, comments) to be injected
into ST4's global prototype context.
*/
func (c *PushDownAutomatonIRConfiguration[TToken, TTokenRole]) AddPrototypeTokenRoles(tokenRoles ...TTokenRole) {
	c.prototypeTokenRoles = append(c.prototypeTokenRoles, tokenRoles...)
}

// ------------------------------------------------------------------ IR ORCHESTRATOR

/*
PushDownAutomatonIR is the IR used by editors that are stack-based. It contains
language name/version, a slice of States (contexts with rules), and a scope extension
appended to all scopes. Downstream consumers (e.g. sublime package) serialize it
to editor-specific formats.
*/
type PushDownAutomatonIR struct {
	LanguageName    string
	LanguageVersion string
	States          []State
	ScopeExtension  string
}

/*
PushDownAutomatonIRCreate builds a PushDownAutomatonIR from the given configuration,
lexing ruleset, and grammar package. It extracts tokens from the ruleset, builds base
states (using token overrides when registered), injects nest states (using nest
overrides when registered), and adds the prototype state for prototype token roles.
*/
func PushDownAutomatonIRCreate[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TToken],
) *PushDownAutomatonIR {
	entryGrammar := grammarPackage.Rules[grammarPackage.EntryRule]
	rootTokens := extractRootWhitelistFromGrammar(entryGrammar)

	tokensInUse, tokenPatternMap, prototypeTokens := extractLexerTokens(lexingRuleSet, config.prototypeTokenRoles)

	allStates, prototypeIncludes := buildBaseStates(config, tokensInUse, tokenPatternMap, prototypeTokens, rootTokens)

	allStates = injectNestStates(config, allStates, grammarPackage, tokenPatternMap)
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
		isRoot := isStructurallyRoot && !isProto

		baseState := State{
			ID:            id,
			Label:         label,
			Rules:         []StateRule{mainRule},
			IsRootContext: isRoot,
		}

		if isProto {
			prototypeIncludes = append(prototypeIncludes, id)
		}

		allStates = append(allStates, baseState)
		allStates = append(allStates, extraStates...)
	}

	return allStates, prototypeIncludes
}

func injectNestStates[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken, TTokenRole],
	allStates []State,
	grammarPackage syntaxa.GrammarPackage[TToken],
	tokenPatternMap map[TToken]Pattern,
) []State {
	getBaseStateID := func(tok TToken) StateID {
		return StateID(produceStateID(sanitizeContextName(config.formatter(tok))))
	}

	for _, nest := range grammarPackage.Nests {
		nestLabel := sanitizeContextName(string(nest.OwnerRule))
		openStateID := getBaseStateID(nest.Open)

		var overrideFn NestOverrideFunc[TToken]
		for _, h := range config.nestOverrideHandlers {
			if h.pred(&nest) {
				overrideFn = h.fn
				break
			}
		}
		if overrideFn != nil {
			getRegEx := func(tok TToken) string {
				p := tokenPatternMap[tok]
				r, _ := p.ToRegEx()
				return r
			}
			ctx := &NestOverrideContext[TToken]{
				NestLabel:      nestLabel,
				ScopeExtension: config.scopeExtension,
				getRegEx:      getRegEx,
			}

			entryStateID, customStates := overrideFn(ctx)
			linkOpenTokenToPushAction(allStates, openStateID, entryStateID)
			allStates = append(allStates, customStates...)
			continue
		}

		bodyStateID := StateID(produceStateID(nestLabel + "_body"))

		linkOpenTokenToPushAction(allStates, getBaseStateID(nest.Open), bodyStateID)

		validTokens := extractWhitelistFromNest(nest.Node)
		includes := buildIncludesWhitelist(validTokens, nest.Open, nest.Close, getBaseStateID)

		closeRegex, _ := tokenPatternMap[nest.Close].ToRegEx()
		metaScopeBase := fmt.Sprintf("meta.block.%s", strings.ToLower(nestLabel))

		nestState := State{
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

		allStates = append(allStates, nestState)
	}

	return allStates
}

/*
injectPrototypeState generates the magic 'prototype' state required by ST4.
ST4 automatically injects this state at the top of every context on the stack.
*/
func injectPrototypeState(allStates []State, prototypeIncludes []StateID) []State {
	if len(prototypeIncludes) == 0 {
		return allStates
	}

	protoState := State{
		ID:            StateID(produceStateID("prototype")),
		Label:         "prototype", // MUST be named exactly "prototype"
		IsRootContext: false,
		Includes:      prototypeIncludes,
	}

	return append(allStates, protoState)
}

/*
injectMainState adds a "main" state that includes all root contexts and an invalid
fallback rule after those includes. So at the root level, invalid constructs are
highlighted; the Sublime generator emits this state as contexts["main"] when present.
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

// ------------------------------------------------------------------ UTILITY EXTRACTORS

func linkOpenTokenToPushAction(states []State, openStateID, targetBodyID StateID) {
	for i := range states {
		if states[i].ID == openStateID && len(states[i].Rules) > 0 {
			states[i].Rules[0].Action = ACTION_PUSH
			states[i].Rules[0].ActionTarget = targetBodyID
			return
		}
	}
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

// ------------------------------------------------------------------ INVALID FALLBACK HELPER

const invalidFallbackLabel = "invalid_fallback"

const invalidFallbackBaseScope = "invalid.illegal.unexpected-token"

/*
InvalidFallbackRule returns a StateRule that matches any non-whitespace run and applies
the invalid.illegal scope. Use it so the highlighter marks unexpected tokens in a state.
The Sublime generator places rules with this label last (after includes). Pass a stable
rule ID and the scope extension (e.g. config.scopeExtension or ctx.ScopeExtension).
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

// ------------------------------------------------------------------ PRIVATE HELPERS

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

	case syntaxa.GEpsilon:
		// No tokens consumed
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
