package bootstrap

import (
	"fmt"
	"langspec"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"langspec/toolchain"
	"lexarch"
	"memarch"
)

// ----------------------------------------------------------------- CONFIGURATION

// ParserCompiler holds bootstrap settings for a single TNodeKind (~uint32) used by
// dsl.LangSpecCompilerCompile[TNodeKind] and matching toolchain entry points.
type ParserCompiler[TNodeKind ~uint32] struct {
	specFile                string
	allocFn                 memarch.AllocationFn
	diagnosticSink          *dsl.LangSpecDiagnosticSink
	lexerPatternCompilerSet bool
	lexerPatternCompiler    lexarch.PatternCompilerMode

	lexerPositionSet      bool
	lexerPositionTracking dsl.LangSpecLexerPositionTracking
	lexerRuneTabWidth     int

	nodePoolPrefillSet bool
	nodePoolPrefill    int
	nodePoolGrowFn     func(currentCap, needed int) int

	// Pipeline filters
	toolchainFilter []string

	// Sublime Config
	sublimeManifest *toolchain.SemanticManifest[string, string]
	sublimeFactory  toolchain.SublimeInMemoryOverrideFactory[TNodeKind]
	fileExtensions  []string
	scopeExtension  string

	// TM comments: nil = derive from PRAGMA via TMCommentsConfigurationFromPragma
	tmCommentsOverride *toolchain.TMCommentsConfiguration
}

// Option configures ParserCompiler[TNodeKind]. TNodeKind must match LangSpecCompilerCompile's type argument.
type Option[TNodeKind ~uint32] func(*ParserCompiler[TNodeKind])

/* WithDiagnosticSink sets compile diagnostics output for DSL compilation. */
func WithDiagnosticSink[TNodeKind ~uint32](sink *dsl.LangSpecDiagnosticSink) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.diagnosticSink = sink
	}
}

/* WithLexerPatternCompiler sets lexer pattern compiler mode for parser compilation. */
func WithLexerPatternCompiler[TNodeKind ~uint32](mode lexarch.PatternCompilerMode) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.lexerPatternCompilerSet = true
		c.lexerPatternCompiler = mode
	}
}

/* WithLexerPositionTrackingGeneric sets generic lexarch position tracking for compiled lexer specs. */
func WithLexerPositionTrackingGeneric[TNodeKind ~uint32]() Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.lexerPositionSet = true
		c.lexerPositionTracking = dsl.LangSpecLexerPositionTrackingGeneric
	}
}

/*
WithLexerPositionTrackingRuneFast sets the rune fast position kernel with the given tab width.
tabWidth must be greater than zero.
*/
func WithLexerPositionTrackingRuneFast[TNodeKind ~uint32](tabWidth int) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		if tabWidth <= 0 {
			panic("langspec/bootstrap: WithLexerPositionTrackingRuneFast requires tabWidth > 0")
		}
		c.lexerPositionSet = true
		c.lexerPositionTracking = dsl.LangSpecLexerPositionTrackingRuneFast
		c.lexerRuneTabWidth = tabWidth
	}
}

/* WithLexerPositionTrackingCompilerDefault restores the LangSpec compiler default (rune fast, tab 4). */
func WithLexerPositionTrackingCompilerDefault[TNodeKind ~uint32]() Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.lexerPositionSet = true
		c.lexerPositionTracking = dsl.LangSpecLexerPositionTrackingCompilerDefault
		c.lexerRuneTabWidth = 0
	}
}

/*
WithNodePoolPrefill sets a fixed Syntaxa LST node pool prefill on the LangParser (see langspec.LangParserConfiguration.WithNodePoolPrefill).

When unset, LangParser derives a hint from loaded source length when building the parse context.
*/
func WithNodePoolPrefill[TNodeKind ~uint32](hint int) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		if hint < 0 {
			panic("langspec/bootstrap: WithNodePoolPrefill requires hint >= 0")
		}
		c.nodePoolPrefillSet = true
		c.nodePoolPrefill = hint
	}
}

/*
WithNodePoolGrowFn sets syntaxa.SyntaxaParser.SetNodePoolGrowFn on the produced LangParser (nil = default).

growFn(currentCap, needed) matches memforge.GrowthStrategy: cap(nodeFree) and minimum
length after growth; return target capacity >= needed.
*/
func WithNodePoolGrowFn[TNodeKind ~uint32](growFn func(currentCap, needed int) int) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.nodePoolGrowFn = growFn
	}
}

/*
	WithToolchainFilter restricts the execution to a specific list of toolchain names

(e.g., "go_bindings", "sublime", "tm_comments"). If never called, all toolchains are executed.
*/
func WithToolchainFilter[TNodeKind ~uint32](toolchainNames ...string) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.toolchainFilter = toolchainNames
	}
}

/* WithSublimeToolchain configures the pipeline to use an in-memory Sublime manifest. */
func WithSublimeToolchain[TNodeKind ~uint32](
	manifest toolchain.SemanticManifest[string, string],
	factory toolchain.SublimeInMemoryOverrideFactory[TNodeKind],
	fileExtensions []string,
	scopeExtension string,
) Option[TNodeKind] {
	return func(c *ParserCompiler[TNodeKind]) {
		c.sublimeManifest = &manifest
		c.sublimeFactory = factory
		c.fileExtensions = fileExtensions
		c.scopeExtension = scopeExtension
	}
}

/*
WithTMCommentsToolchain supplies TMCommentsConfiguration for the tm_comments toolchain, bypassing
PRAGMA-derived scope and comment strings. When unset, RunToolchainsFrom* uses tool.tm_comments
settings from the compiled spec (scope or scope-extension, single-line-comment-start, optional block pair).
*/
func WithTMCommentsToolchain[TNodeKind ~uint32](cfg toolchain.TMCommentsConfiguration) Option[TNodeKind] {
	copyCfg := cfg
	return func(c *ParserCompiler[TNodeKind]) {
		c.tmCommentsOverride = &copyCfg
	}
}

// ----------------------------------------------------------------- BOOTSTRAP PIPELINE

/*
CompileParserFromSpec compiles a .lspec file and returns a LangParser ready to parse
source. It runs the DSL compiler only.
*/
func CompileParserFromSpec[TNodeKind ~uint32](
	specFile string,
	alloc memarch.AllocationFn,
	opts ...Option[TNodeKind],
) (*LangParser[TNodeKind], error) {
	p, _, err := CompileParserFromSpecWithCompiledSymbols[TNodeKind](specFile, alloc, opts...)
	return p, err
}

/*
CompileParserFromSpecWithCompiledSymbols is like CompileParserFromSpec but also returns
the CompiledSymbolTable that maps uint32 token, role, and node-kind IDs to names for the
compiled target language. Use it for LST debug output and other ID-to-string resolution.
*/
func CompileParserFromSpecWithCompiledSymbols[TNodeKind ~uint32](
	specFile string,
	alloc memarch.AllocationFn,
	opts ...Option[TNodeKind],
) (*LangParser[TNodeKind], *semantics.CompiledSymbolTable, error) {
	cfg := buildConfig(specFile, alloc, opts...)

	compileResult, err := compileDSL[TNodeKind](cfg)
	if err != nil {
		return nil, nil, err
	}

	return createParser(cfg, compileResult), compileResult.CompiledSymbols, nil
}

/*
RunToolchainsFromSpecFile compiles the .lspec at specFile and runs toolchains per PRAGMA and filters.
*/
func RunToolchainsFromSpecFile[TNodeKind ~uint32](specFile string, alloc memarch.AllocationFn, opts ...Option[TNodeKind]) error {
	cfg := buildConfig(specFile, alloc, opts...)
	compileResult, err := compileDSL[TNodeKind](cfg)
	if err != nil {
		return err
	}
	return executePipeline(cfg, compileResult)
}

/*
RunToolchainsFromCompileResult runs toolchain steps on an existing compile result.
*/
func RunToolchainsFromCompileResult[TNodeKind ~uint32](
	compileResult *dsl.LangSpecCompileResult[TNodeKind],
	opts ...Option[TNodeKind],
) error {
	cfg := &ParserCompiler[TNodeKind]{}
	for _, opt := range opts {
		opt(cfg)
	}
	return executePipeline(cfg, compileResult)
}

// ----------------------------------------------------------------- SINGLE-RESPONSIBILITY HELPERS

func buildConfig[TNodeKind ~uint32](specFile string, alloc memarch.AllocationFn, opts ...Option[TNodeKind]) *ParserCompiler[TNodeKind] {
	cfg := &ParserCompiler[TNodeKind]{
		specFile: specFile,
		allocFn:  alloc,
	}
	for _, opt := range opts {
		opt(cfg)
	}
	return cfg
}

func (c *ParserCompiler[TNodeKind]) shouldRunToolchain(name string) bool {
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

func compileDSL[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind]) (*dsl.LangSpecCompileResult[TNodeKind], error) {
	compilerConfig := dsl.LangSpecCompilerConfigurationCreate(cfg.allocFn, nil)
	if cfg.diagnosticSink != nil {
		compilerConfig.WithDiagnosticSink(cfg.diagnosticSink)
	}
	if cfg.lexerPatternCompilerSet {
		compilerConfig.WithLexerPatternCompiler(cfg.lexerPatternCompiler)
	}
	if cfg.lexerPositionSet {
		switch cfg.lexerPositionTracking {
		case dsl.LangSpecLexerPositionTrackingGeneric:
			compilerConfig.WithLexerPositionTrackingGeneric()
		case dsl.LangSpecLexerPositionTrackingRuneFast:
			compilerConfig.WithLexerPositionTrackingRuneFast(cfg.lexerRuneTabWidth)
		case dsl.LangSpecLexerPositionTrackingCompilerDefault:
			compilerConfig.WithLexerPositionTrackingCompilerDefault()
		}
	}

	langSpecCompiler := dsl.LangSpecCompilerCreate(compilerConfig)
	defer dsl.LangSpecCompilerDestroy(langSpecCompiler)

	result, err := dsl.LangSpecCompilerCompile[TNodeKind](langSpecCompiler, cfg.specFile)
	if err != nil {
		return nil, fmt.Errorf("DSL compilation failed: %w", err)
	}

	if cfg.diagnosticSink != nil && cfg.diagnosticSink.Writer != nil {
		fmt.Fprintf(cfg.diagnosticSink.Writer, "Successfully created compiler for '%s @ %s' using LangSpec %s.\n\n", result.LanguageName, result.LanguageVersion, result.TargetLangspecVersion)
	}

	return result, nil
}

func createParser[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind], compileResult *dsl.LangSpecCompileResult[TNodeKind]) *LangParser[TNodeKind] {
	langSpec := langspec.LangSpecCreate(
		compileResult.CompiledLexerSpec,
		compileResult.CompiledParserSpec,
	)

	parserConfig := langspec.LangParserConfigurationCreate(langSpec, cfg.allocFn)
	if cfg.nodePoolPrefillSet {
		parserConfig = parserConfig.WithNodePoolPrefill(cfg.nodePoolPrefill)
	}
	if cfg.nodePoolGrowFn != nil {
		parserConfig = parserConfig.WithNodePoolGrowFn(cfg.nodePoolGrowFn)
	}
	return langspec.LangParserCreate(parserConfig)
}

// ----------------------------------------------------------------- PIPELINE EXECUTION

func executePipeline[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind], compileResult *dsl.LangSpecCompileResult[TNodeKind]) error {
	if err := executeGoBindings(cfg, compileResult); err != nil {
		return fmt.Errorf("go bindings failed: %w", err)
	}

	if err := executeSublime(cfg, compileResult); err != nil {
		return fmt.Errorf("sublime execution failed: %w", err)
	}

	if err := executeTMComments(cfg, compileResult); err != nil {
		return fmt.Errorf("tm_comments execution failed: %w", err)
	}

	return nil
}

func executeGoBindings[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind], compileResult *dsl.LangSpecCompileResult[TNodeKind]) error {
	if !cfg.shouldRunToolchain(toolchain.GoBindingsToolName) {
		return nil
	}
	return toolchain.RunGoBindingsToolchain(compileResult)
}

func executeSublime[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind], compileResult *dsl.LangSpecCompileResult[TNodeKind]) error {
	if !cfg.shouldRunToolchain(toolchain.SublimeToolName) {
		return nil
	}

	if cfg.sublimeManifest == nil {
		return toolchain.RunSublimeToolchain(compileResult, nil)
	}

	return runInMemorySublime(cfg, compileResult)
}

func executeTMComments[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind], compileResult *dsl.LangSpecCompileResult[TNodeKind]) error {
	if !cfg.shouldRunToolchain(toolchain.TmCommentsToolName) {
		return nil
	}

	pragma, ok := compileResult.CompiledToolPragmas[toolchain.TmCommentsToolName]
	if !ok {
		return nil
	}

	enabled, ok := pragma.Settings[toolchain.TmCommentsEnabledKey].(string)
	if !ok || enabled != "true" {
		return nil
	}

	var tmCfg toolchain.TMCommentsConfiguration
	var err error
	if cfg.tmCommentsOverride != nil {
		tmCfg = *cfg.tmCommentsOverride
	} else {
		tmCfg, err = toolchain.TMCommentsConfigurationFromPragma(pragma)
		if err != nil {
			return err
		}
	}

	return toolchain.RunTMCommentsToolchain(compileResult, tmCfg)
}

func runInMemorySublime[TNodeKind ~uint32](cfg *ParserCompiler[TNodeKind], compileResult *dsl.LangSpecCompileResult[TNodeKind]) error {
	pragma, ok := compileResult.CompiledToolPragmas[toolchain.SublimeToolName]
	if !ok {
		return nil
	}

	if pragma.Settings[toolchain.SublimeEnableKey].(string) != "true" {
		return nil
	}

	outputPaths, err := toolchain.ExtractOutputPathsFromPragma(pragma, toolchain.SublimeOutputPathKey)
	if err != nil {
		return fmt.Errorf("could not extract output path: %w", err)
	}

	return dispatchInMemorySublimeGeneration(cfg, compileResult, outputPaths)
}

func dispatchInMemorySublimeGeneration[TNodeKind ~uint32](
	cfg *ParserCompiler[TNodeKind],
	compileResult *dsl.LangSpecCompileResult[TNodeKind],
	outputPaths []string,
) error {
	if compileResult.CompiledSymbols == nil {
		return fmt.Errorf("bootstrap sublime: compiled symbols missing")
	}
	return toolchain.RunSublimeToolchainFromMemory[TNodeKind](
		compileResult,
		*cfg.sublimeManifest,
		cfg.sublimeFactory,
		outputPaths,
		cfg.fileExtensions,
		cfg.scopeExtension,
	)
}
