// Package tests holds LangSpec DSL integration checks.
//
// Maintainer workflow: regenerating the example .lspec from the live grammar (generator) is
// exercised separately from the client path. Client-style coverage uses bootstrap.CompileParserFromSpec
// (parser only) and, where toolchains are needed, bootstrap.RunToolchainsFromCompileResult.
package tests
