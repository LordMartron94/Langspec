package editor

import (
	"autarch/pattern"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"lexarch"
	"strings"
	"syntaxa"
)

var xxh3Hasher = hash.XXH3HasherCreateWithSeed(42)

// ------------------------------------------------------------------ TYPES

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
*/
//go:generate stringer -type RuleAction
type RuleAction uint8

const (
	ACTION_PUSH RuleAction = iota
	ACTION_POP
	ACTION_SET
	ACTION_MATCH
	ACTION_NONE
)

type State struct {
	ID            StateID
	Label         string
	MetaScope     string
	Rules         []StateRule
	Includes      []StateID
	IsRootContext bool
}

type StateRule struct {
	ID           StateRuleID
	Label        string
	RegEx        string
	Scope        string
	Action       RuleAction
	ActionTarget StateID
	Captures     map[int]string
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

// ------------------------------------------------------------------ CONFIGURATION

type PushDownAutomatonIRConfiguration[TToken comparable] struct {
	scopeProvider  ScopeProvider[TToken]
	formatter      TokenFormatter[TToken]
	scopeExtension string
	overrides      map[TToken]TokenOverrideFunc
}

func PushDownAutomatonIRConfigurationCreate[TToken comparable](
	provider ScopeProvider[TToken],
	formatter TokenFormatter[TToken],
	scopeExtension string,
) *PushDownAutomatonIRConfiguration[TToken] {
	return &PushDownAutomatonIRConfiguration[TToken]{
		scopeProvider:  provider,
		formatter:      formatter,
		scopeExtension: scopeExtension,
		overrides:      make(map[TToken]TokenOverrideFunc),
	}
}

func (c *PushDownAutomatonIRConfiguration[TToken]) AddOverride(token TToken, fn TokenOverrideFunc) {
	c.overrides[token] = fn
}

// ------------------------------------------------------------------ IR ORCHESTRATOR

/*
PushDownAutomatonIR is the IR used by editors that are stack-based.
*/
type PushDownAutomatonIR struct {
	LanguageName    string
	LanguageVersion string
	States          []State
	ScopeExtension  string
}

func PushDownAutomatonIRCreate[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TToken],
) *PushDownAutomatonIR {

	tokensInUse, tokenPatternMap := extractLexerTokens(lexingRuleSet)
	allStates := buildBaseStates(config, tokensInUse, tokenPatternMap)
	allStates = injectNestStates(config, allStates, grammarPackage, tokenPatternMap)

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
) ([]TToken, map[TToken]Pattern) {
	var tokensInUse []TToken
	tokenPatternMap := make(map[TToken]Pattern)

	for _, lexerRule := range lexingRuleSet.GetRules() {
		tokensInUse = append(tokensInUse, lexerRule.Token)
		tokenPatternMap[lexerRule.Token] = lexerRule.Pattern
	}
	return tokensInUse, tokenPatternMap
}

func buildBaseStates[TToken comparable](
	config *PushDownAutomatonIRConfiguration[TToken],
	tokensInUse []TToken,
	tokenPatternMap map[TToken]Pattern,
) []State {
	var allStates []State

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

		baseState := State{
			ID:            id,
			Label:         label,
			Rules:         []StateRule{mainRule},
			IsRootContext: true,
		}

		allStates = append(allStates, baseState)
		allStates = append(allStates, extraStates...)
	}

	return allStates
}

func injectNestStates[TToken comparable](
	config *PushDownAutomatonIRConfiguration[TToken],
	allStates []State,
	grammarPackage syntaxa.GrammarPackage[TToken],
	tokenPatternMap map[TToken]Pattern,
) []State {
	getBaseStateID := func(tok TToken) StateID {
		return StateID(produceStateID(config.formatter(tok)))
	}

	for _, nest := range grammarPackage.Nests {
		nestLabel := sanitizeContextName(string(nest.OwnerRule))
		bodyStateID := StateID(produceStateID(nestLabel + "_body"))

		linkOpenTokenToPushAction(allStates, getBaseStateID(nest.Open), bodyStateID)

		validTokens := extractWhitelistFromNest(nest.Node)
		includes := buildIncludesWhitelist(validTokens, nest.Open, nest.Close, getBaseStateID)

		closeRegex, _ := tokenPatternMap[nest.Close].ToRegEx()

		nestState := State{
			ID:            bodyStateID,
			Label:         nestLabel + "_body",
			IsRootContext: false,
			MetaScope:     getScopeString(config.scopeProvider(nest.Open)+".block", config.scopeExtension),
			Includes:      includes,
			Rules: []StateRule{
				{
					ID:     StateRuleID(produceStateID(nestLabel + "_close")),
					Label:  nestLabel + "_close",
					RegEx:  closeRegex,
					Scope:  getScopeString(config.scopeProvider(nest.Close), config.scopeExtension),
					Action: ACTION_POP,
				},
				{
					ID:     StateRuleID(produceStateID(nestLabel + "_invalid")),
					Label:  "invalid_fallback",
					RegEx:  `\S+`,
					Scope:  "invalid.illegal.unexpected-token" + config.scopeExtension,
					Action: ACTION_MATCH,
				},
			},
		}

		allStates = append(allStates, nestState)
	}

	return allStates
}

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
