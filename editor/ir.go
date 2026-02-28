package editor

import (
	"autarch/pattern"
	"fmt"
	"foundation/bytes"
	"foundation/hash"
	"lexarch"
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
	ID    StateID
	Label string

	MetaScope string
	Rules     []StateRule

	IsRootContext bool
}

type StateRule struct {
	ID    StateRuleID
	Label string

	RegEx        string
	Scope        string
	Action       RuleAction
	ActionTarget StateID
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

// ------------------------------------------------------------------ IR

/*
PushDownAutomatonIR is the IR used by editors that are stack-based.
*/
type PushDownAutomatonIR struct {
	LanguageName    string
	LanguageVersion string

	States []State

	ScopeExtension string
}

func PushDownAutomatonIRCreate[TToken, TTokenRole comparable](
	config *PushDownAutomatonIRConfiguration[TToken],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	grammarPackage syntaxa.GrammarPackage[TToken],
) *PushDownAutomatonIR {
	tokensInUse := make([]TToken, 0)
	tokenPatternMap := make(map[TToken]Pattern)

	for _, lexerRule := range lexingRuleSet.GetRules() {
		tokensInUse = append(tokensInUse, lexerRule.Token)
		tokenPatternMap[lexerRule.Token] = lexerRule.Pattern
	}

	var allStates []State

	for _, token := range tokensInUse {
		label := config.formatter(token)
		id := StateID(produceStateID(label))
		baseScope := config.scopeProvider(token)

		var mainRule StateRule
		var extraStates []State

		// Check for client override
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
			// Default behavior
			defaultRegex, _ := tokenPatternMap[token].ToRegEx()
			mainRule = StateRule{
				ID:     StateRuleID(id),
				Label:  label,
				Action: ACTION_MATCH,
				Scope:  getScopeString(baseScope, config.scopeExtension),
				RegEx:  defaultRegex,
			}
		}

		// Create the base state for this token
		baseState := State{
			ID:            id,
			Label:         label,
			Rules:         []StateRule{mainRule},
			IsRootContext: true,
		}

		allStates = append(allStates, baseState)
		allStates = append(allStates, extraStates...)
	}

	return &PushDownAutomatonIR{
		LanguageName:    grammarPackage.Name,
		LanguageVersion: grammarPackage.Version,
		States:          allStates,
		ScopeExtension:  config.scopeExtension,
	}
}

// ------------------------------------------------------------------ PRIVATE HELPERS

func getScopeString(baseScope, scopeExtension string) string {
	return fmt.Sprintf("%s%s", baseScope, scopeExtension)
}

func produceStateID(stateLabel string) uint64 {
	bytes := bytes.StringSliceToBytes([]string{stateLabel}, ';')
	id := hash.XXH3HasherHash64(xxh3Hasher, bytes)
	return id
}
