// Package editor provides the editor IR and construction for LangSpec-based editor/IDE integration.
//
// The main IR type is PushDownAutomatonIR: a stack-based representation of states (contexts),
// each with rules that match patterns and perform push/pop/set/match actions. It holds
// language name, version, a slice of States, and a scope extension. Use
// PushDownAutomatonIRConfiguration to configure scope provider, token formatter, prototype
// token roles, and overrides; then PushDownAutomatonIRCreate to build the IR from a
// syntaxa GrammarPackage and lexarch LexingRuleset.
//
// Override mechanism: token overrides replace the default single-rule state for a token
// with a custom StateRule and optional extra States (e.g. delimited regions). Nest
// overrides replace the default single-body state for a grammar nest with a custom
// state sequence. Use the structural (semantic-agnostic) helpers in this package—
// TokenOverrideDelimitedRegion, TokenOverrideMatchWithCapture, BuildNestStateSequence—
// to build overrides from regex and scope strings without constructing State/StateRule
// by hand. The editor package does not prescribe language semantics; clients supply
// regex and scope strings and choose which helper fits each token or nest.
package editor
