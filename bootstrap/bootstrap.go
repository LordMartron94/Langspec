package bootstrap

import (
	"fmt"
	"langspec"
	"langspec/dsl"
	"langspec/editor"
	"langspec/toolchain"
	"memarch"
)

// ----------------------------------------------------------------- CONFIGURATION

/*
ParserCompiler holds configuration for the bootstrap pipeline (spec path, allocator,
optional diagnostic sink, optional Sublime override producer). Built by buildConfig
from CompileParserFromSpec options; not constructed directly by callers.
*/
type ParserCompiler struct {
	specFile        string
	allocFn         memarch.AllocationFn
	diagnosticSink  *dsl.LangSpecDiagnosticSink
	sublimeOverride func(ec *editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool)
}

/*
Option configures a ParserCompiler when passed to CompileParserFromSpec.
*/
type Option func(*ParserCompiler)

/*
WithDiagnosticSink sets the diagnostic sink for compiler output (e.g. trace, validation).
If nil, no diagnostics are written. Use dsl.DefaultLangSpecDiagnosticSink() for stdout.
*/
func WithDiagnosticSink(sink *dsl.LangSpecDiagnosticSink) Option {
	return func(c *ParserCompiler) {
		c.diagnosticSink = sink
	}
}

/*
WithSublimeOverrides sets the override producer used when the Sublime toolchain runs.

When the .lspec PRAGMA enables the Sublime tool, RunSublimeToolchain is invoked with
this producer. Pass the result of OverrideRegistry.Producer() (after registering
handlers, e.g. from TextPatternBuilder) to supply token overrides for syntax generation.
If unset, the Sublime toolchain uses no overrides when enabled.
*/
func WithSublimeOverrides(overrideFn func(ec *editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool)) Option {
	return func(c *ParserCompiler) {
		c.sublimeOverride = overrideFn
	}
}

// ----------------------------------------------------------------- BOOTSTRAP PIPELINE

/*
CompileParserFromSpec compiles a .lspec file and returns a LangParser ready to parse source.

Pipeline: build config from opts → compile DSL (LangSpecCompilerCompile) → run toolchains
(e.g. Sublime if enabled in PRAGMA and WithSublimeOverrides was set) → create parser from
CompiledLexerSpec and CompiledParserSpec. specFile must be a path to a .lspec file; alloc
is used for compiler and parser allocation. Returns (nil, error) on compile or toolchain failure.
*/
func CompileParserFromSpec(
	specFile string,
	alloc memarch.AllocationFn,
	opts ...Option,
) (*langspec.LangParser[rune, string, string, string, string], error) {
	cfg := buildConfig(specFile, alloc, opts...)

	compileResult, err := compileDSL(cfg)
	if err != nil {
		return nil, err
	}

	if err := runToolchains(cfg, compileResult); err != nil {
		return nil, err
	}

	return createParser(cfg, compileResult), nil
}

// ----------------------------------------------------------------- SINGLE-RESPONSIBILITY HELPERS

func buildConfig(
	specFile string,
	alloc memarch.AllocationFn,
	opts ...Option,
) *ParserCompiler {

	cfg := &ParserCompiler{
		specFile: specFile,
		allocFn:  alloc,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func compileDSL(
	cfg *ParserCompiler,
) (*dsl.LangSpecCompileResult, error) {
	compilerConfig := dsl.LangSpecCompilerConfigurationCreate(cfg.allocFn, nil)
	if cfg.diagnosticSink != nil {
		compilerConfig.WithDiagnosticSink(cfg.diagnosticSink)
	}

	langSpecCompiler := dsl.LangSpecCompilerCreate(compilerConfig)
	defer dsl.LangSpecCompilerDestroy(langSpecCompiler)

	result, err := dsl.LangSpecCompilerCompile(langSpecCompiler, cfg.specFile)
	if err != nil {
		return nil, fmt.Errorf("DSL compilation failed: %w", err)
	}

	fmt.Fprintf(cfg.diagnosticSink.Writer, "Successfully created compiler for '%s @ %s' using LangSpec %s.\n\n", result.LanguageName, result.LanguageVersion, result.TargetLangspecVersion)

	return result, nil
}

func runToolchains(
	cfg *ParserCompiler,
	compileResult *dsl.LangSpecCompileResult,
) error {

	if cfg.sublimeOverride == nil {
		return nil
	}

	err := toolchain.RunSublimeToolchain(compileResult, cfg.sublimeOverride)
	if err != nil {
		return fmt.Errorf("sublime toolchain execution failed: %w", err)
	}
	return nil
}

func createParser(
	cfg *ParserCompiler,
	compileResult *dsl.LangSpecCompileResult,
) *langspec.LangParser[rune, string, string, string, string] {

	langSpec := langspec.LangSpecCreate(
		compileResult.CompiledLexerSpec,
		compileResult.CompiledParserSpec,
	)

	parserConfig := langspec.LangParserConfigurationCreate(langSpec, cfg.allocFn)
	return langspec.LangParserCreate(parserConfig)
}
