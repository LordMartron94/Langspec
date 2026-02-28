# LangSpec

LangSpec is a language specification and parsing ecosystem: it defines a meta-language (the LangSpec DSL) for describing lexers, grammars, and validation, and provides tooling to parse and validate source files and to generate editor integrations.

## Subpackages

- **Root package (`langspec`)** — Public API: language specs, parser configuration, and file-based parsing.
- **`dsl`** — The LangSpec DSL and its compiler. The meta-language used to describe language specs (lexer rules, parser grammar, validation stages). Source files (`.lspec`) are compiled via `LangSpecCompilerCompile`, which returns a structured `LangSpecCompileResult` (AST, trace, syntax errors, validation entries). Diagnostic output (syntax errors, trace, AST dump) is optional and directed through `LangSpecDiagnosticSink` (e.g. `DefaultLangSpecDiagnosticSink()` for stdout). The dsl package does not depend on editor or sublime.
- **`dsl/editor`** — Editor integration for the LangSpec DSL itself: builds Sublime Text syntax definitions from a compiler instance (`BuildSublimeSyntaxForDSL`). The DSL is the consumer of the editor IR; this subpackage lives under dsl for that reason.
- **`editor`** — Editor IR (stack-based push-down automaton) and construction. Provides semantic-agnostic helpers (delimited regions, match-with-capture, nest state sequences) so clients can build syntax-highlighting IR without hand-constructing low-level states.
- **`editor/sublime`** — Generates Sublime Text YAML syntax definitions from the editor IR.
- **`validation`** — AST validation stages and result reporting.

## LangSpec DSL

The DSL is the meta-language that describes a language: token types, lexer rules, grammar rules, and validation. A `.lspec` file is parsed and compiled by the LangSpec DSL compiler. `LangSpecCompilerCompile` returns a `LangSpecCompileResult` with the parsed AST, parse trace, syntax errors, and validation entries. Use `WithDiagnosticSink(DefaultLangSpecDiagnosticSink())` on the compiler configuration to emit human-readable diagnostics to stdout. For Sublime Text syntax generation, use BuildSublimeSyntaxForDSL from package `langspec/dsl/editor`. See the `dsl` package and `dsl/compiler.go` for the public API.
