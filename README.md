# LangSpec

LangSpec is a language specification and parsing ecosystem: it defines a meta-language (the LangSpec DSL) for describing lexers, grammars, and validation, and provides tooling to parse and validate source files and to generate editor integrations.

## Subpackages

- **Root package (`langspec`)** — Public API: language specs, parser configuration, and file-based parsing.
- **`dsl`** — The LangSpec DSL and its compiler. The meta-language used to describe language specs (lexer rules, parser grammar, validation stages). Source files (`.lspec`) are compiled into runtime LangSpec structures. Editor integration (e.g. Sublime syntax) is built from the same grammar via the editor IR.
- **`editor`** — Editor IR (stack-based push-down automaton) and construction. Provides semantic-agnostic helpers (delimited regions, match-with-capture, nest state sequences) so clients can build syntax-highlighting IR without hand-constructing low-level states.
- **`editor/sublime`** — Generates Sublime Text YAML syntax definitions from the editor IR.
- **`validation`** — AST validation stages and result reporting.

## LangSpec DSL (initial)

The DSL is the meta-language that describes a language: token types, lexer rules, grammar rules, and validation. A `.lspec` file is parsed and compiled by the LangSpec DSL compiler into a `LangSpec` used by the parser. This README will be extended with DSL syntax and usage; for now, see the `dsl` package and the compiler in `dsl/compiler.go`.
