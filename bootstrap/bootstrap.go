package bootstrap

import (
	"fmt"
	"langspec"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"langspec/editor"
	"langspec/toolchain"
	"lexarch"
	"memarch"
)

// ----------------------------------------------------------------- CONFIGURATION

type SublimeOverrideFactory func(
	ruleset *editor.LexingRuleSet[rune, uint32, uint32],
	ctxProducer func(ctx *editor.EditorCtx[rune, uint32, uint32, string, uint32]) toolchain.SublimeContext,
) func(ec *editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, toolchain.SublimeContext]

type ParserCompiler struct {
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
	sublimeFactory  SublimeOverrideFactory
	fileExtensions  []string
	scopeExtension  string

	// TM comments: nil = derive from PRAGMA via TMCommentsConfigurationFromPragma
	tmCommentsOverride *toolchain.TMCommentsConfiguration
}

type Option func(*ParserCompiler)

/* WithDiagnosticSink sets compile diagnostics output for DSL compilation. */
func WithDiagnosticSink(sink *dsl.LangSpecDiagnosticSink) Option {
	return func(c *ParserCompiler) {
		c.diagnosticSink = sink
	}
}

/* WithLexerPatternCompiler sets lexer pattern compiler mode for parser compilation. */
func WithLexerPatternCompiler(mode lexarch.PatternCompilerMode) Option {
	return func(c *ParserCompiler) {
		c.lexerPatternCompilerSet = true
		c.lexerPatternCompiler = mode
	}
}

/* WithLexerPositionTrackingGeneric sets generic lexarch position tracking for compiled lexer specs. */
func WithLexerPositionTrackingGeneric() Option {
	return func(c *ParserCompiler) {
		c.lexerPositionSet = true
		c.lexerPositionTracking = dsl.LangSpecLexerPositionTrackingGeneric
	}
}

/*
WithLexerPositionTrackingRuneFast sets the rune fast position kernel with the given tab width.
tabWidth must be greater than zero.
*/
func WithLexerPositionTrackingRuneFast(tabWidth int) Option {
	return func(c *ParserCompiler) {
		if tabWidth <= 0 {
			panic("langspec/bootstrap: WithLexerPositionTrackingRuneFast requires tabWidth > 0")
		}
		c.lexerPositionSet = true
		c.lexerPositionTracking = dsl.LangSpecLexerPositionTrackingRuneFast
		c.lexerRuneTabWidth = tabWidth
	}
}

/* WithLexerPositionTrackingCompilerDefault restores the LangSpec compiler default (rune fast, tab 4). */
func WithLexerPositionTrackingCompilerDefault() Option {
	return func(c *ParserCompiler) {
		c.lexerPositionSet = true
		c.lexerPositionTracking = dsl.LangSpecLexerPositionTrackingCompilerDefault
		c.lexerRuneTabWidth = 0
	}
}

/*
WithNodePoolPrefill sets a fixed Syntaxa LST node pool prefill on the LangParser (see langspec.LangParserConfiguration.WithNodePoolPrefill).

When unset, LangParser derives a hint from loaded source length when building the parse context.
*/
func WithNodePoolPrefill(hint int) Option {
	return func(c *ParserCompiler) {
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
func WithNodePoolGrowFn(growFn func(currentCap, needed int) int) Option {
	return func(c *ParserCompiler) {
		c.nodePoolGrowFn = growFn
	}
}

/*
	WithToolchainFilter restricts the execution to a specific list of toolchain names

(e.g., "go_bindings", "sublime", "tm_comments"). If never called, all toolchains are executed.
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

/*
WithTMCommentsToolchain supplies TMCommentsConfiguration for the tm_comments toolchain, bypassing
PRAGMA-derived scope and comment strings. When unset, RunToolchainsFrom* uses tool.tm_comments
settings from the compiled spec (scope or scope-extension, single-line-comment-start, optional block pair).
*/
func WithTMCommentsToolchain(cfg toolchain.TMCommentsConfiguration) Option {
	copyCfg := cfg
	return func(c *ParserCompiler) {
		c.tmCommentsOverride = &copyCfg
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
) (*LangParser, error) {
	p, _, err := CompileParserFromSpecWithCompiledSymbols(specFile, alloc, opts...)
	return p, err
}

/*
CompileParserFromSpecWithCompiledSymbols is like CompileParserFromSpec but also returns
the CompiledSymbolTable that maps uint32 token, role, and node-kind IDs to names for the
compiled target language. Use it for LST debug output and other ID-to-string resolution.
*/
func CompileParserFromSpecWithCompiledSymbols(
	specFile string,
	alloc memarch.AllocationFn,
	opts ...Option,
) (*LangParser, *semantics.CompiledSymbolTable, error) {
	cfg := buildConfig(specFile, alloc, opts...)

	compileResult, err := compileDSL(cfg)
	if err != nil {
		return nil, nil, err
	}

	return createParser(cfg, compileResult), compileResult.CompiledSymbols, nil
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

	result, err := dsl.LangSpecCompilerCompile(langSpecCompiler, cfg.specFile)
	if err != nil {
		return nil, fmt.Errorf("DSL compilation failed: %w", err)
	}

	if cfg.diagnosticSink != nil && cfg.diagnosticSink.Writer != nil {
		fmt.Fprintf(cfg.diagnosticSink.Writer, "Successfully created compiler for '%s @ %s' using LangSpec %s.\n\n", result.LanguageName, result.LanguageVersion, result.TargetLangspecVersion)
	}

	return result, nil
}

func createParser(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) *LangParser {
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

func executePipeline(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
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

func executeGoBindings(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
	if !cfg.shouldRunToolchain(toolchain.GoBindingsToolName) {
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

func executeTMComments(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
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

func runInMemorySublime(cfg *ParserCompiler, compileResult *dsl.LangSpecCompileResult) error {
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

func dispatchInMemorySublimeGeneration(
	cfg *ParserCompiler,
	compileResult *dsl.LangSpecCompileResult,
	outputPaths []string,
) error {
	if compileResult.CompiledSymbols == nil {
		return fmt.Errorf("bootstrap sublime: compiled symbols missing")
	}
	var factory toolchain.SublimeInMemoryOverrideFactory
	if cfg.sublimeFactory != nil {
		factory = toolchain.SublimeInMemoryOverrideFactory(cfg.sublimeFactory)
	}
	return toolchain.RunSublimeToolchainFromMemory(
		compileResult,
		*cfg.sublimeManifest,
		factory,
		outputPaths,
		cfg.fileExtensions,
		cfg.scopeExtension,
	)
}
