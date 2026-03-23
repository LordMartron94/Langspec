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

type SublimeOverrideFactory func(
	ruleset *lexarch.LexingRuleset[rune, string, string],
	ctxProducer func(ctx *editor.EditorCtx[rune, string, string, string, string]) toolchain.SublimeContext,
) func(ec *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]

type ParserCompiler struct {
	specFile       string
	allocFn        memarch.AllocationFn
	diagnosticSink *dsl.LangSpecDiagnosticSink

	// Pipeline filters
	toolchainFilter []string

	// Sublime Config
	sublimeManifest *toolchain.SemanticManifest[string, string]
	sublimeFactory  SublimeOverrideFactory
	fileExtensions  []string
	scopeExtension  string
}

type Option func(*ParserCompiler)

/* WithDiagnosticSink sets compile diagnostics output for DSL compilation. */
func WithDiagnosticSink(sink *dsl.LangSpecDiagnosticSink) Option {
	return func(c *ParserCompiler) {
		c.diagnosticSink = sink
	}
}

/*
	WithToolchainFilter restricts the execution to a specific list of toolchain names

(e.g., "go_bindings", "sublime"). If never called, all toolchains are executed.
*/
func WithToolchainFilter(toolchainNames ...string) Option {
	return func(c *ParserCompiler) {
		c.toolchainFilter = toolchainNames
	}
}

/* WithSublimeToolchain configures the pipeline to use an in-memory Sublime manifest. */
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
CompileParserFromSpec compiles a .lspec file and returns a LangParser ready to parse
source. It runs the DSL compiler only.
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

	return createParser(cfg, compileResult), nil
}

/*
RunToolchainsFromSpecFile compiles the .lspec at specFile and runs toolchains per PRAGMA and filters.
*/
func RunToolchainsFromSpecFile(specFile string, alloc memarch.AllocationFn, opts ...Option) error {
	cfg := buildConfig(specFile, alloc, opts...)
	compileResult, err := compileDSL(cfg)
	if err != nil {
		return err
	}
	return executePipeline(cfg, compileResult)
}

/*
RunToolchainsFromCompileResult runs toolchain steps on an existing compile result.
*/
func RunToolchainsFromCompileResult(
	compileResult *dsl.LangSpecCompileResult,
	opts ...Option,
) error {
	cfg := &ParserCompiler{}
	for _, opt := range opts {
		opt(cfg)
	}
	return executePipeline(cfg, compileResult)
}

// ----------------------------------------------------------------- SINGLE-RESPONSIBILITY HELPERS

func buildConfig(specFile string, alloc memarch.AllocationFn, opts ...Option) *ParserCompiler {
	cfg := &ParserCompiler{
		specFile: specFile,
		allocFn:  alloc,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func (c *ParserCompiler) shouldRunToolchain(name string) bool {
	if len(c.toolchainFilter) == 0 {
		return true // No filter means run all
	}
	for _, filter := range c.toolchainFilter {
		if filter == name {
			return true
		}
	}
	return false
}

func compileDSL(cfg *ParserCompiler) (*dsl.LangSpecCompileResult, error) {
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

	if cfg.diagnosticSink != nil && cfg.diagnosticSink.Writer != nil {
		fmt.Fprintf(cfg.diagnosticSink.Writer, "Successfully created compiler for '%s @ %s' using LangSpec %s.\n\n", result.LanguageName, result.LanguageVersion, result.TargetLangspecVersion)
	}

	return result, nil
}

func createParser(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) *langspec.LangParser[rune, string, string, string, string] {
	langSpec := langspec.LangSpecCreate(
		compileResult.CompiledLexerSpec,
		compileResult.CompiledParserSpec,
	)

	parserConfig := langspec.LangParserConfigurationCreate(langSpec, cfg.allocFn)
	return langspec.LangParserCreate(parserConfig)
}

// ----------------------------------------------------------------- PIPELINE EXECUTION

func executePipeline(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
	if err := executeGoBindings(cfg, compileResult); err != nil {
		return fmt.Errorf("go bindings failed: %w", err)
	}

	if err := executeSublime(cfg, compileResult); err != nil {
		return fmt.Errorf("sublime execution failed: %w", err)
	}

	return nil
}

func executeGoBindings(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
	if !cfg.shouldRunToolchain("go_bindings") {
		return nil
	}
	return toolchain.RunGoBindingsToolchain(compileResult)
}

func executeSublime(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
	if !cfg.shouldRunToolchain(toolchain.SublimeToolName) {
		return nil
	}

	if cfg.sublimeManifest == nil {
		return toolchain.RunSublimeToolchain(compileResult, nil)
	}

	return runInMemorySublime(cfg, compileResult)
}

func runInMemorySublime(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
	pragma := findSublimePragma(compileResult.CompiledToolPragmas)
	if pragma == nil {
		return nil
	}

	if pragma.Settings[toolchain.SublimeEnableKey].(string) != "true" {
		return nil
	}

	outputPaths, err := toolchain.ExtractOutputPathsFromSublimePragma(*pragma)
	if err != nil {
		return fmt.Errorf("could not extract output path: %w", err)
	}

	return dispatchInMemorySublimeGeneration(cfg, compileResult, outputPaths)
}

func findSublimePragma(pragmas []dsl.ToolPragma) *dsl.ToolPragma {
	for _, pragma := range pragmas {
		if pragma.ToolName == toolchain.SublimeToolName {
			return &pragma
		}
	}
	return nil
}

func dispatchInMemorySublimeGeneration(
	cfg *ParserCompiler,
	compileResult *dsl.LangSpecCompileResult,
	outputPaths []string,
) error {
	ruleset := compileResult.CompiledLexerSpec.Ruleset("default")
	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, string, string, string](*cfg.sublimeManifest)

	var overrideProducer func(ec *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]
	if cfg.sublimeFactory != nil {
		overrideProducer = cfg.sublimeFactory(&ruleset, ctxProducer)
	}

	return toolchain.RunSublimeToolchainFromMemory(
		compileResult,
		*cfg.sublimeManifest,
		overrideProducer,
		outputPaths,
		cfg.fileExtensions,
		cfg.scopeExtension,
	)
}
