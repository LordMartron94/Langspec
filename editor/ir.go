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

	Rules []StateRule
}

type StateRule struct {
	ID    StateRuleID
	Label string

	RegEx  string
	Scope  string
	Action RuleAction
}

type ScopeProvider[T any] func(item T) string

type TokenFormatter[TToken any] func(token TToken) string

// ------------------------------------------------------------------ CONFIGURATION

type PushDownAutomatonIRConfiguration[TToken any] struct {
	scopeProvider  ScopeProvider[TToken]
	formatter      TokenFormatter[TToken]
	scopeExtension string
}

func PushDownAutomatonIRConfigurationCreate[TToken any](
	provider ScopeProvider[TToken],
	formatter TokenFormatter[TToken],
	scopeExtension string,
) *PushDownAutomatonIRConfiguration[TToken] {
	return &PushDownAutomatonIRConfiguration[TToken]{
		scopeProvider:  provider,
		formatter:      formatter,
		scopeExtension: scopeExtension,
	}
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

	states := make([]State, len(tokensInUse))
	for i, token := range tokensInUse {
		label := config.formatter(token)
		id := produceStateID(label)
		defaultRegex, _ := tokenPatternMap[token].ToRegEx()

		states[i] = State{
			ID:    StateID(id),
			Label: label,
			Rules: []StateRule{
				{
					ID:     StateRuleID(id),
					Label:  label,
					Action: ACTION_MATCH,
					Scope:  getScopeString(config.scopeProvider(token), config.scopeExtension),
					RegEx:  defaultRegex,
				},
			},
		}
	}

	// TODO - RegEx overrides optional by client

	return &PushDownAutomatonIR{
		LanguageName:    grammarPackage.Name,
		LanguageVersion: grammarPackage.Version,
		States:          states,
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
