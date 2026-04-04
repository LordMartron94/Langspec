package editor

import (
	"autarch/pattern"
	"cmp"
	"langspec"
)

/*
LexingRule describes one lexer rule used by langspec editor IR generation.

This local type decouples editor/toolchain surfaces from lexarch-internal rule
container types.
*/
type LexingRule[TObservation cmp.Ordered, TToken, TTokenRole comparable] struct {
	Token    TToken
	Role     TTokenRole
	Pattern  pattern.RegulaAST[TObservation]
	Priority int

	LexerState string

	StackKind      langspec.LexerStackOpKind
	StackTargets   []string
	StackPopAmount int
}

/* LexingRuleSet is an ordered collection of LexingRule entries. */
type LexingRuleSet[TObservation cmp.Ordered, TToken, TTokenRole comparable] struct {
	rules []LexingRule[TObservation, TToken, TTokenRole]
}

/* LexingRuleSetCreate creates a rule set preserving the given order. */
func LexingRuleSetCreate[TObservation cmp.Ordered, TToken, TTokenRole comparable](
	rules ...LexingRule[TObservation, TToken, TTokenRole],
) *LexingRuleSet[TObservation, TToken, TTokenRole] {
	out := &LexingRuleSet[TObservation, TToken, TTokenRole]{
		rules: make([]LexingRule[TObservation, TToken, TTokenRole], len(rules)),
	}
	copy(out.rules, rules)
	return out
}

/* LexingRuleSetGetRules returns a copy of all rules in stable order. */
func LexingRuleSetGetRules[TObservation cmp.Ordered, TToken, TTokenRole comparable](
	ruleset *LexingRuleSet[TObservation, TToken, TTokenRole],
) []LexingRule[TObservation, TToken, TTokenRole] {
	if ruleset == nil || len(ruleset.rules) == 0 {
		return nil
	}

	out := make([]LexingRule[TObservation, TToken, TTokenRole], len(ruleset.rules))
	copy(out, ruleset.rules)
	return out
}
