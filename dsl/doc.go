// Package dsl defines the LangSpec domain-specific language and its compilation pipeline.
//
// The DSL is a self-hosted meta-language used to describe complete language specifications,
// including lexer rules, parser grammars, AST contracts, and validation stages.
//
// Source files written in the DSL are parsed using the LangSpec engine itself and compiled
// into runtime LangSpec structures (LexerSpec, ParserSpec, and LangSpec) that drive the
// actual parsing pipeline.
//
// In effect, this package forms the bootstrap layer of LangSpec:
//
//	DSL source  →  DSL AST  →  compiled LangSpec runtime structures
//
// This architecture enables:
//
//   - full separation of language semantics from execution mechanics
//   - self-hosting and introspection of the language definition system
//   - reusable compilation passes and validation stages
//   - uniform tooling across user languages and the meta-language itself
//
// The DSL is intentionally expressive enough to define:
//
//   - token patterns and lexing state machines
//   - grammar rules and precedence systems
//   - AST node kinds and structural constraints
//   - multi-stage semantic validation pipelines
//
// while remaining decoupled from runtime concerns such as memory allocation,
// streaming configuration, and performance tuning.
//
// Subpackages typically include:
//
//   - ast        — syntax tree representation of the DSL
//   - compiler  — lowering and transformation passes into runtime LangSpec structures
//   - passes    — semantic analysis and validation phases
//
// This package is not a helper API.
// It is the language that defines languages for the LangSpec ecosystem.
package dsl
