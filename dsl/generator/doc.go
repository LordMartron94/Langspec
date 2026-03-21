// Package generator emits a .lspec document from live lexer and grammar data (maintainer tooling).
// It is not used when end users compile an existing .lspec; that path is package dsl
// (LangSpecCompilerCompile). Use GenerateLSpec with a GeneratorConfig to write or refresh a
// checked-in example grammar file from the DSL’s in-memory spec.
package generator
