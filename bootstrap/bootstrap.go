package bootstrap

import (
	"fmt"
	"langspec"
	"langspec/dsl"
	"langspec/editor"
	"langspec/toolchain"
	"lexarch"
	"memarch"
)

// ----------------------------------------------------------------- CONFIGURATION

// SublimeOverrideFactory allows the host to build overrides using compiled dependencies
type SublimeOverrideFactory func(
	ruleset *lexarch.LexingRuleset[rune, string, string],
	ctxProducer func(ctx *editor.EditorCtx[rune, string, string, string, string]) toolchain.SublimeContext,
) func(ec *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]

/*
ParserCompiler holds configuration for the bootstrap pipeline.
Built by buildConfig from CompileParserFromSpec options.
*/
type ParserCompiler struct {
	specFile       string
	allocFn        memarch.AllocationFn
	diagnosticSink *dsl.LangSpecDiagnosticSink

	sublimeManifest *toolchain.SemanticManifest[string, string]
	sublimeFactory  SublimeOverrideFactory
	fileExtensions  []string
	scopeExtension  string
}

type Option func(*ParserCompiler)

func WithDiagnosticSink(sink *dsl.LangSpecDiagnosticSink) Option {
	return func(c *ParserCompiler) {
		c.diagnosticSink = sink
	}
}

/*
WithSublimeToolchain injects the in-memory manifest and override factory.
When used, the Sublime toolchain will use this strongly-typed Go data.
*/
func WithSublimeToolchain(
	manifest toolchain.SemanticManifest[string, string],
	factory SublimeOverrideFactory,
	fileExtensions []string,
	scopeExtension string,
) Option {
	return func(c *ParserCompiler) {
		c.sublimeManifest = &manifest
		c.sublimeFactory = factory
		c.fileExtensions = fileExtensions
		c.scopeExtension = scopeExtension
	}
}

// ----------------------------------------------------------------- BOOTSTRAP PIPELINE

/*
CompileParserFromSpec compiles a .lspec file and returns a LangParser ready to parse source.
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
	// If the host didn't configure Sublime explicitly, fallback to seeing if it was enabled via JSON in the PRAGMA
	if cfg.sublimeManifest == nil {
		return toolchain.RunSublimeToolchain(compileResult, nil)
	}

	// Host configured Sublime in-memory. Check if PRAGMA explicitly enabled it and set an output path.
	var sublimePragma *dsl.ToolPragma
	for _, pragma := range compileResult.CompiledToolPragmas {
		if pragma.ToolName == toolchain.SublimeToolName {
			sublimePragma = &pragma
			break
		}
	}

	if sublimePragma == nil || sublimePragma.Settings[toolchain.SublimeEnableKey] != "true" {
		return nil
	}

	outputPath := sublimePragma.Settings[toolchain.SublimeOutputPathKey]
	if outputPath == "" {
		return fmt.Errorf("sublime toolchain enabled but missing 'output-path'")
	}

	// 1. Fulfill dependencies
	ruleset := compileResult.CompiledLexerSpec.Ruleset("default")
	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, string, string, string](*cfg.sublimeManifest)

	// 2. Execute factory
	var overrideProducer func(ec *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]
	if cfg.sublimeFactory != nil {
		overrideProducer = cfg.sublimeFactory(&ruleset, ctxProducer)
	}

	// 3. Run pure-memory generation
	err := toolchain.RunSublimeToolchainFromMemory(
		compileResult,
		*cfg.sublimeManifest,
		overrideProducer,
		outputPath,
		cfg.fileExtensions,
		cfg.scopeExtension,
	)

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
