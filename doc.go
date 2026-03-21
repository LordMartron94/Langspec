// Package langspec provides functionality to create and parse languages.
//
// The core langspec library only concerns itself with the parsing pipeline:
// raw source -> lexing -> parsing => LST
//
// Subpackage validation provides additional functionality for straightforward post-LST validation.
// Subpackage dsl provides the LangSpec DSL meant for defining grammars to be used with LangSpec core.
// Subpackage editor provides functionality to integrate with editors.
// Codegen from .lspec files (go bindings, Sublime, parser bootstrap) is provided by subpackages
// bootstrap and cliutil; they do not import dsl.
package langspec
