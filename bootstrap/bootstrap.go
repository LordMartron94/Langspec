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

/*
SublimeOverrideFactory is a function that, given the compiled ruleset and context producer,
returns an override producer for the Sublime toolchain. Used when WithSublimeToolchain is set
so the host can supply token overrides (e.g. line/block comment, embedded regions) from typed Go code.
*/
type SublimeOverrideFactory func(
	ruleset *lexarch.LexingRuleset[rune, string, string],
	ctxProducer func(ctx *editor.EditorCtx[rune, string, string, string, string]) toolchain.SublimeContext,
) func(ec *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]

/*
ParserCompiler holds configuration for the bootstrap pipeline (spec file, allocator, diagnostic
sink, optional Sublime in-memory manifest and factory). Built by buildConfig from
CompileParserFromSpec options; not used directly by callers.
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

/* Option configures a ParserCompiler when passed to CompileParserFromSpec or RunToolchainsFromSpecFile. */
type Option func(*ParserCompiler)

/* WithDiagnosticSink sets compile diagnostics output for DSL compilation inside bootstrap. */
func WithDiagnosticSink(sink *dsl.LangSpecDiagnosticSink) Option {
	return func(c *ParserCompiler) {
		c.diagnosticSink = sink
	}
}

/*
WithSublimeToolchain configures RunToolchainsFromSpecFile / RunToolchainsFromCompileResult
to use an in-memory Sublime manifest and override factory. When set,
toolchain.RunSublimeToolchainFromMemory is used instead of loading JSON from
PRAGMA configuration-path.

Use cases:
- Bootstrapped languages with a typed manifest (e.g. from a compiler) and custom overrides in Go.
- Avoiding a separate JSON config file when the manifest is built in code.
- Supplying an override factory that has access to compiled ruleset and context producer.

Prerequisites:
- The .lspec PRAGMA must still include tool.sublime with enable = true and output-path set.
- manifest, fileExtensions, and scopeExtension are used as-is; configuration-path in PRAGMA is ignored.

Edge cases:
- factory may be nil; then no overrides are applied beyond what the manifest provides.
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
CompileParserFromSpec compiles a .lspec file and returns a LangParser ready to parse
source. It runs the DSL compiler only (no go_bindings or Sublime toolchains). Use
RunToolchainsFromSpecFile or RunToolchainsFromCompileResult at codegen time (e.g. //go:generate).

Prerequisites:
- specFile must be a path to a valid .lspec file; alloc must be a valid allocation function.

Options such as WithDiagnosticSink apply; WithSublimeToolchain is ignored here because
toolchains are not run (use RunToolchainsFromSpecFile for Sublime/go bindings generation).
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
RunToolchainsFromSpecFile compiles the .lspec at specFile and runs toolchains (go bindings,
then Sublime) per PRAGMA and options. Use from //go:generate or build scripts; do not use
for runtime parser creation.

Options: WithDiagnosticSink; WithSublimeToolchain for in-memory manifest (same as RunToolchainsFromCompileResult).
Without WithSublimeToolchain, Sublime uses JSON at PRAGMA configuration-path when enabled.
*/
func RunToolchainsFromSpecFile(specFile string, alloc memarch.AllocationFn, opts ...Option) error {
	cfg := buildConfig(specFile, alloc, opts...)
	compileResult, err := compileDSL(cfg)
	if err != nil {
		return err
	}
	return runToolchains(cfg, compileResult)
}

/*
RunGoBindingsFromSpecFile compiles the .lspec and runs only the go_bindings toolchain per PRAGMA.
WithSublimeToolchain is ignored. Use as the first //go:generate step when a host package defines
manifests or other code that references generated Token/Node types from tool.go_bindings output
before that file exists.

Options: WithDiagnosticSink only (same compile path as RunToolchainsFromSpecFile).
*/
func RunGoBindingsFromSpecFile(specFile string, alloc memarch.AllocationFn, opts ...Option) error {
	cfg := buildConfig(specFile, alloc, opts...)
	compileResult, err := compileDSL(cfg)
	if err != nil {
		return err
	}
	return toolchain.RunGoBindingsToolchain(compileResult)
}

/*
RunToolchainsFromCompileResult runs go_bindings and sublime toolchain steps (go bindings first,
then sublime) on an existing compile result. Use when you already have LangSpecCompilerCompile
output; spec file and allocator are not used here.
*/
func RunToolchainsFromCompileResult(
	compileResult *dsl.LangSpecCompileResult,
	opts ...Option,
) error {
	cfg := &ParserCompiler{}
	for _, opt := range opts {
		opt(cfg)
	}
	return runToolchains(cfg, compileResult)
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

	if cfg.diagnosticSink != nil && cfg.diagnosticSink.Writer != nil {
		fmt.Fprintf(cfg.diagnosticSink.Writer, "Successfully created compiler for '%s @ %s' using LangSpec %s.\n\n", result.LanguageName, result.LanguageVersion, result.TargetLangspecVersion)
	}

	return result, nil
}

func runToolchains(
	cfg *ParserCompiler,
	compileResult *dsl.LangSpecCompileResult,
) error {
	// 1. Run the Go Bindings Toolchain first (it has no external dependencies)
	if err := toolchain.RunGoBindingsToolchain(compileResult); err != nil {
		return fmt.Errorf("go bindings toolchain execution failed: %w", err)
	}

	// 2. Run the Sublime Toolchain
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

	if sublimePragma == nil || sublimePragma.Settings[toolchain.SublimeEnableKey].(string) != "true" {
		return nil
	}

	outputPaths, err := toolchain.ExtractOutputPathsFromSublimePragma(*sublimePragma)
	if err != nil {
		return fmt.Errorf("could not extract output path: %w", err)
	}

	// Fulfill dependencies
	ruleset := compileResult.CompiledLexerSpec.Ruleset("default")
	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, string, string, string](*cfg.sublimeManifest)

	// Execute factory
	var overrideProducer func(ec *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]
	if cfg.sublimeFactory != nil {
		overrideProducer = cfg.sublimeFactory(&ruleset, ctxProducer)
	}

	// Run pure-memory generation
	err = toolchain.RunSublimeToolchainFromMemory(
		compileResult,
		*cfg.sublimeManifest,
		overrideProducer,
		outputPaths,
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
