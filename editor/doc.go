// Package editor provides the editor IR and construction for LangSpec-based editor/IDE integration.
//
// The main IR type is PushDownAutomatonIR: a stack-based representation of states (contexts),
// each with rules that match patterns and perform push/pop/set/match actions. It holds
// language name, version, a slice of States, and a scope extension. Use
// PushDownAutomatonIRConfiguration to configure scope provider, token formatter, prototype
// token roles, and overrides; then PushDownAutomatonIRCreate to build the IR from a
// syntaxa GrammarPackage and lexarch LexingRuleset.
//
// Types:
//   - State: one context in the push-down automaton (ID, label, rules, includes, root flag).
//   - StateRule: one match rule (regex, scope, action, optional capture/embed/escape).
//   - StateID / StateRuleID: opaque identifiers for states and rules.
//   - RuleAction: ACTION_PUSH, ACTION_POP, ACTION_SET, ACTION_MATCH, ACTION_NONE, ACTION_EMBED.
//
// Delimited tokens: tokens that have a DelimitedRule in the LexingRuleset (open/close
// patterns) are mapped automatically to ST4-style regions (open rule PUSHes to a body
// state, body state has a close rule that POPs). No override is required for such tokens.
//
// Override mechanism: token overrides replace the default single-rule state for a token
// with a custom StateRule and optional extra States (e.g. line comment with capture).
// Use OverrideRegistry to map tokens to OverrideHandlers and pass Registry.Producer()
// as the overrideProducer in EditorIRConfiguration. TextPatternBuilder provides
// default patterns (LineComment, BlockComment) that return handlers you can register
// for line- and block-comment tokens. Nest overrides replace the default single-body
// state for a grammar nest with a custom state sequence. The editor package does not
// prescribe language semantics; clients supply regex and scope strings.
package editor
