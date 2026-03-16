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

type ParserCompiler struct {
	specFile        string
	allocFn         memarch.AllocationFn
	diagnosticSink  *dsl.LangSpecDiagnosticSink
	sublimeOverride func(ec *editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool)
}

type Option func(*ParserCompiler)

func WithDiagnosticSink(sink *dsl.LangSpecDiagnosticSink) Option {
	return func(c *ParserCompiler) {
		c.diagnosticSink = sink
	}
}

func WithSublimeOverrides(overrideFn func(ec *editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool)) Option {
	return func(c *ParserCompiler) {
		c.sublimeOverride = overrideFn
	}
}

// ----------------------------------------------------------------- BOOTSTRAP PIPELINE

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
