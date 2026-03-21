// Package validation provides a configurable pipeline for validating a parsed Lossless Syntax Tree
// after lexing and parsing complete. It is part of langspec core and does not depend on the
// LangSpec DSL (langspec/dsl).
//
// Build stages with LSTValidationStageCreate, assemble an LSTValidatorConfiguration with
// LSTValidatorConfigurationCreate (register stages, optional pre-pass), then run LSTValidatorRun
// on the root LST node produced by the core parser. Use this for semantic checks on your target
// language that cannot be expressed in the grammar alone.
//
// For validating .lspec files as LangSpec DSL sources, see dsl/semantics and the DSL compiler’s
// built-in validation pipeline instead.
package validation
