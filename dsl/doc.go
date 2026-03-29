// Package dsl defines the LangSpec domain-specific language and its compilation pipeline.
//
// The DSL is a self-hosted meta-language used to describe complete language specifications,
// including lexer rules, parser grammars, LST contracts, and validation stages.
//
// Source files written in the DSL are parsed using the LangSpec engine itself and compiled
// into runtime LangSpec structures (LexerSpec, ParserSpec, and LangSpec) that drive the
// actual parsing pipeline.
//
// In effect, this package forms the bootstrap layer of LangSpec:
//
//	DSL source  →  DSL LST  →  compiled LangSpec runtime structures
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
//   - LST node kinds and structural constraints
//   - multi-stage semantic validation pipelines
//
// while remaining decoupled from runtime concerns such as memory allocation
// and performance tuning.
//
// Public API: LangSpecCompilerConfiguration (with optional LangSpecDiagnosticSink),
// LangSpecCompilerCreate, LangSpecCompilerCompile (returns LangSpecCompileResult),
// LangSpecCompilerDestroy. Diagnostic output is optional; when a sink is set,
// syntax errors, parse trace, validation entries, and LST dumps are written to it.
//
// Editor integration for the DSL (e.g. Sublime Text syntax generation) lives in
// subpackage dsl/editor, which consumes this package and the generic editor IR.
//
// Related packages:
//   - dsl/spec: meta-language definition (token/node kinds, self-hosted lexer/parser grammar for .lspec)
//   - dsl/semantics: semantic environment and LST validation stages for the DSL
//   - dsl/generator: emits a checked-in .lspec from the live grammar (maintainer tooling)
package dsl
