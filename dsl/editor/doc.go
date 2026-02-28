// Package editor provides editor integrations for the LangSpec DSL. The DSL is a
// consumer of the langspec/editor IR and langspec/editor/sublime generator:
// this package builds Sublime Text syntax definitions from a LangSpec compiler
// instance. It depends on the parent dsl package (token types, grammar IDs,
// compiler accessors), langspec/editor (IR), and langspec/editor/sublime (YAML).
package editor
