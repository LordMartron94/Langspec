// Package semantics provides the semantic environment (symbol tables from the DSL LST) and
// multi-stage LST validation for LangSpec .lspec sources. It depends on dsl/spec for node kinds
// and AST shape; it does not compile specs to runtime lexer/parser structures (package dsl).
package semantics
