// Package langspec provides functionality to create and parse languages.
//
// The core langspec library only concerns itself with the parsing pipeline:
// raw source -> lexing -> parsing => AST
//
// Subpackage validation provides additional functionality for straightforward post-AST validation.
// Subpackage dsl provides the LangSpec DSL meant for defining grammars to be used with LangSpec core.
// Subpackage editor provides functionality to integrate with editors.
package langspec
