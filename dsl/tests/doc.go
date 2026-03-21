// Package tests holds LangSpec DSL integration checks.
//
// Maintainer workflow: regenerating the example .lspec from the live grammar (generator) is
// exercised separately from the client path. Client-style coverage uses bootstrap.CompileParserFromSpec
// and bootstrap.RunToolchainsFromCompileResult so the same entry points as applications are tested.
package tests
