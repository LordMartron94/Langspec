package dsl

import (
	"autarch/pattern"
	"fmt"
	"foundation/system"
	"io"
	"langspec"
	"langspec/dsl/semantics"
	dslspec "langspec/dsl/spec"
	"langspec/validation"
	"lexarch"
	"memarch"
	"syntaxa"
	"syntaxa/lowering"
)

// --------------------------------------------------------------- TYPE ALIASES

/* LexingRuleset is the lexing ruleset type for the LangSpec DSL lexer (rune observations). */
type LexingRuleset = lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]

/* ValidationStage is a validation stage for the DSL LST when grammar package state is not yet available. */
type ValidationStage = validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]

/* ValidationStageCtx is the per-stage context for early DSL validation passes. */
type ValidationStageCtx = validation.LSTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]

/* NodeFinalizationCtx is the syntaxa finalization context for DSL parse nodes. */
type NodeFinalizationCtx = syntaxa.FinalizationCtx[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

/* AttributeAs reads a typed attribute from a DSL LST node, delegating to syntaxa.AttributeAs. */
func AttributeAs[TAttribute any](node *Node, attributeName string) (TAttribute, bool) {
	return syntaxa.AttributeAs[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, TAttribute](
		node, attributeName,
	)
}

// --------------------------------------------------------------- CONFIGURATION

/*
LangSpecLexerPositionTracking selects how LangSpecCompiler configures lexarch position tracking
on compiled lexer specs.

LangSpecLexerPositionTrackingCompilerDefault is the built-in choice (rune fast kernel, tab width 4).

LangSpecLexerPositionTrackingGeneric uses generic newline/column callbacks (no fast kernel).

LangSpecLexerPositionTrackingRuneFast uses the rune fast kernel; tab width is set via
WithLexerPositionTrackingRuneFast.
*/
type LangSpecLexerPositionTracking int

const (
	LangSpecLexerPositionTrackingCompilerDefault LangSpecLexerPositionTracking = iota
	LangSpecLexerPositionTrackingGeneric
	LangSpecLexerPositionTrackingRuneFast
)

/* LangSpecCompilerConfiguration encapsulates the configuration for the langspec compiler. */
type LangSpecCompilerConfiguration struct {
	scratchAllocationFunction memarch.AllocationFn
	stageReporter             validation.LSTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]
	diagnosticSink            *LangSpecDiagnosticSink
	lexerScanConfig           lexarch.LexerScanConfig

	lexerPositionTracking LangSpecLexerPositionTracking
	lexerRuneTabWidth     int

	// collectParseTraceForLspec enables syntaxa parse trace when parsing .lspec sources (high allocation cost).
	collectParseTraceForLspec bool
}

/*
LangSpecCompilerConfigurationCreate creates an instance of the compiler configuration.

Stage reporter is optional. DiagnosticSink is optional; when set, compilation diagnostics
(syntax errors, trace, validation, LST dump) are written to the sink's Writer.
*/
func LangSpecCompilerConfigurationCreate(
	scratchAllocationFunction memarch.AllocationFn,
	stageReporter validation.LSTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState],
) *LangSpecCompilerConfiguration {
	return &LangSpecCompilerConfiguration{
		scratchAllocationFunction: scratchAllocationFunction,
		stageReporter:             stageReporter,
		diagnosticSink:            nil,
		lexerScanConfig:           lexarch.LexerScanConfigDefault(),
	}
}

/*
WithDiagnosticSink configures where compilation diagnostics are written.
When nil, no diagnostic output is produced. Use DefaultLangSpecDiagnosticSink() for stdout.
*/
func (c *LangSpecCompilerConfiguration) WithDiagnosticSink(sink *LangSpecDiagnosticSink) *LangSpecCompilerConfiguration {
	c.diagnosticSink = sink
	return c
}

/* WithLexerScanMode sets lexarch scan mode for compiled language lexers. */
func (c *LangSpecCompilerConfiguration) WithLexerScanMode(mode lexarch.LexerScanMode) *LangSpecCompilerConfiguration {
	c.lexerScanConfig.Mode = mode
	return c
}

/* WithLexerScanConfig sets full lexarch scan configuration for compiled language lexers. */
func (c *LangSpecCompilerConfiguration) WithLexerScanConfig(cfg lexarch.LexerScanConfig) *LangSpecCompilerConfiguration {
	c.lexerScanConfig = cfg
	return c
}

/*
WithLexerPositionTrackingGeneric selects generic lexarch position tracking for compiled lexer specs
(no rune/byte fast kernel).
*/
func (c *LangSpecCompilerConfiguration) WithLexerPositionTrackingGeneric() *LangSpecCompilerConfiguration {
	c.lexerPositionTracking = LangSpecLexerPositionTrackingGeneric
	return c
}

/*
WithLexerPositionTrackingRuneFast selects the rune fast position kernel with the given tab width.
tabWidth must be greater than zero.
*/
func (c *LangSpecCompilerConfiguration) WithLexerPositionTrackingRuneFast(tabWidth int) *LangSpecCompilerConfiguration {
	if tabWidth <= 0 {
		panic("langspec/dsl: WithLexerPositionTrackingRuneFast requires tabWidth > 0")
	}
	c.lexerPositionTracking = LangSpecLexerPositionTrackingRuneFast
	c.lexerRuneTabWidth = tabWidth
	return c
}

/*
WithLexerPositionTrackingCompilerDefault restores the compiler built-in position tracking
(rune fast kernel, tab width 4).
*/
func (c *LangSpecCompilerConfiguration) WithLexerPositionTrackingCompilerDefault() *LangSpecCompilerConfiguration {
	c.lexerPositionTracking = LangSpecLexerPositionTrackingCompilerDefault
	c.lexerRuneTabWidth = 0
	return c
}

/*
WithCollectParseTraceForLspec enables full parse trace collection while compiling .lspec files.

When false (default), LangSpecCompileResult.Trace may be nil and LangSpecCompilerDebugResult with DebugParseTrace shows no event data unless traces are collected. Enable when debugging the LangSpec parser.
*/
func (c *LangSpecCompilerConfiguration) WithCollectParseTraceForLspec(v bool) *LangSpecCompilerConfiguration {
	c.collectParseTraceForLspec = v
	return c
}

// --------------------------------------------------------------- COMPILER

/*
LangSpecCompileResult holds the result of compiling a .lspec file: the parsed LST,
parse trace, syntax errors (if any), and validation entries (when validation was run).
Callers can inspect the result without parsing stdout.
*/
type LangSpecCompileResult struct {
	RootNode          *Node
	Trace             *syntaxa.ParseTrace[LangSpecLexerTokenType]
	SyntaxErrors      *syntaxa.SyntaxErrors[rune]
	ValidationEntries *validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

	LanguageName    string
	LanguageVersion string

	CompiledLexerSpec  *LexerSpec
	CompiledParserSpec *ParserSpec

	TargetLangspecVersion string

	CompiledGrammarPackage GrammarPackage

	CompiledToolPragmas []ToolPragma
	SourceMap           map[*syntaxa.Grammar[uint32, uint32]]*Node

	CompiledSymbols *semantics.CompiledSymbolTable
	EOFToken        uint32
}

/*
LangSpecCompiler compiles a .lspec file into the LangSpec configuration needed by the LangParser.
*/
type LangSpecCompiler struct {
	config *LangSpecCompilerConfiguration

	languageSpec LanguageSpec

	parser          *langspec.LangParser[rune, LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	validatorConfig *validation.LSTValidatorConfiguration[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]

	lexingRuleSet *lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]
	programRule   Rule

	sessionCache *langspec.LangParserSession[rune]

	diagnosticWriter io.Writer
}

/* LangSpecCompilerCreate constructs a compiler instance. */
func LangSpecCompilerCreate(compilerConfig *LangSpecCompilerConfiguration) *LangSpecCompiler {
	languageSpec, dslSpec, ruleset, programRule := dslspec.BuildLangSpecDSLSpec()

	langParserConfig := langspec.LangParserConfigurationCreate(
		dslSpec,
		compilerConfig.scratchAllocationFunction,
	)
	if compilerConfig.collectParseTraceForLspec {
		langParserConfig = langParserConfig.WithCollectParseTrace(true)
	}
	parser := langspec.LangParserCreate(langParserConfig)

	// Register ALL stages (0 through 4) in the unified config
	validationConfig := validation.LSTValidatorConfigurationCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]()
	validationConfig = validationConfig.WithStageReporter(compilerConfig.stageReporter).WithStages(
		semantics.ValidationStages()...,
	)

	var w io.Writer
	if compilerConfig.diagnosticSink != nil && compilerConfig.diagnosticSink.Writer != nil {
		w = compilerConfig.diagnosticSink.Writer
	}

	return &LangSpecCompiler{
		config:           compilerConfig,
		languageSpec:     languageSpec,
		parser:           parser,
		validatorConfig:  validationConfig,
		lexingRuleSet:    ruleset,
		programRule:      programRule,
		diagnosticWriter: w,
	}
}

/*
LangSpecCompilerDestroy destroys the compiler.

Forgetting to call this results in memory leaks.
*/
func LangSpecCompilerDestroy(compiler *LangSpecCompiler) {
	langspec.LangParserDestroy(compiler.parser)
}

/*
LangSpecCompilerCompile compiles a .lspec file and returns a structured result plus an error.
When the file is not .lspec or lexing fails, result is nil. Otherwise result is populated
with the parsed LST, trace, syntax errors, and validation entries (if validation ran).
Diagnostic output is written only when config's DiagnosticSink is set.

It does not execute PRAGMA toolchains (go_bindings, Sublime, etc.); those require a separate
codegen step such as bootstrap.RunToolchainsFromSpecFile using the same spec path or a
LangSpecCompileResult from bootstrap.RunToolchainsFromCompileResult.
*/
func LangSpecCompilerCompile(
	compiler *LangSpecCompiler,
	sourceFile string,
) (*LangSpecCompileResult, error) {
	if !system.PathHasExt(sourceFile, ".lspec") {
		return nil, fmt.Errorf("file is not a .lspec file: %s", sourceFile)
	}

	contentRune, _ := system.FileReadAllRunes(sourceFile)
	session := langSpecCompilerSessionGet(compiler, sourceFile)

	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(
		compiler.parser,
		session,
	)

	// 1. Guard against fatal parser initialization failures (where syntaxErrors is nil)
	if syntaxErrors == nil {
		if err != nil {
			return nil, fmt.Errorf("parser initialization failed: %w", err)
		}
		return nil, fmt.Errorf("fatal internal error: parser returned nil syntax errors without an error")
	}

	result := &LangSpecCompileResult{
		RootNode:     rootNode,
		Trace:        trace,
		SyntaxErrors: syntaxErrors,
	}

	// 2. Handle syntax errors
	if syntaxErrors.HasErrors() {
		RenderSyntaxErrorsWithContext(compiler.diagnosticWriter, contentRune, syntaxErrors, lexarch.ColumnAdvanceRune(4))
		return result, fmt.Errorf("langspec parse failed with %d syntax errors", len(syntaxErrors.Errors))
	}

	// 3. First Validation Pass
	validationEntries, validationErr := validation.LSTValidatorRun(
		compiler.validatorConfig,
		rootNode,
		func(stage *validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]) bool {
			return stage.Order < 4
		},
		(*semantics.GrammarValidationState)(nil),
	)

	result.ValidationEntries = validationEntries

	if validationErr != nil {
		return result, validationErr
	}

	if hasCriticalValidationErrors(validationEntries) {
		renderValidationEntries(compiler.diagnosticWriter, contentRune, validationEntries, lexarch.ColumnAdvanceRune(4))
		return result, fmt.Errorf("parsing failed with validation errors")
	}

	// 4. Compilation & Post-Validation
	compiled := compileTree(compiler, result.RootNode)

	valState := &semantics.GrammarValidationState{
		Package:   &compiled.grammarPackage,
		SourceMap: compiled.sourceMap,
		Symbols:   compiled.symbols,
	}

	postValidationEntries, postValidationErr := validation.LSTValidatorRun(
		compiler.validatorConfig,
		rootNode,
		func(stage *validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, *semantics.GrammarValidationState]) bool {
			return stage.Order >= 4
		},
		valState,
	)

	if result.ValidationEntries == nil {
		result.ValidationEntries = postValidationEntries
	} else if postValidationEntries != nil {
		result.ValidationEntries.Results = append(result.ValidationEntries.Results, postValidationEntries.Results...)
	}

	if postValidationErr != nil {
		return result, postValidationErr
	}

	if hasCriticalValidationErrors(postValidationEntries) {
		renderValidationEntries(compiler.diagnosticWriter, contentRune, postValidationEntries, lexarch.ColumnAdvanceRune(4))
		return result, fmt.Errorf("compilation failed with grammar safety errors")
	}

	// 5. Finalize Result
	result.LanguageName = compiled.dslName
	result.LanguageVersion = compiled.dslVersion
	result.CompiledLexerSpec = compiled.lexerSpec
	result.CompiledParserSpec = compiled.parserSpec
	result.CompiledGrammarPackage = compiled.grammarPackage
	result.CompiledToolPragmas = compiled.toolPragmas
	result.TargetLangspecVersion = compiled.targetLangspecVersion
	result.CompiledSymbols = compiled.symbols
	result.EOFToken = compiled.eofToken
	result.SourceMap = compiled.sourceMap

	return result, nil
}

// Helper method to keep the main function clean
func hasCriticalValidationErrors(entries *validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]) bool {
	if entries == nil {
		return false
	}
	for _, stage := range entries.Results {
		for _, entry := range stage.Entries {
			if entry.Severity > validation.VALIDATION_SEVERITY_INFO {
				return true
			}
		}
	}
	return false
}

/* CompilerDebugConfig selects optional debug output for LangSpecCompilerDebugResult. */
type CompilerDebugConfig struct {
	DebugParseTrace bool
	DebugLST        bool
}

/* LangSpecCompilerDebugLexemes lexes sourceFile and writes lexeme debug output to the compiler diagnostic writer. */
func LangSpecCompilerDebugLexemes(compiler *LangSpecCompiler, sourceFile string) error {
	session := langSpecCompilerSessionGet(compiler, sourceFile)

	lexemes, err := langspec.LangParserLexFile(compiler.parser, session)
	if err != nil {
		return fmt.Errorf("lexing error: %w", err)
	}

	renderLexemes(compiler.diagnosticWriter, lexemes,
		func(t LangSpecLexerTokenType) string { return t.String() },
		func(r LangSpecLexerTokenRole) string { return r.String() },
	)

	return nil
}

/* LangSpecCompilerDebugGrammar writes grammar and grammar-package debug dumps to the diagnostic writer. */
func LangSpecCompilerDebugGrammar(compiler *LangSpecCompiler) {
	grammarDump := compiler.programRule.GetGrammar().DebugDump(
		syntaxa.GrammarDebugFormatter[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			FormatKind:           syntaxa.GrammarKind.String,
			FormatToken:          LangSpecLexerTokenType.String,
			FormatOutputNodeKind: LangSpecParserNodeKind.String,
			FormatRange: func(min int, max *int) string {
				if max == nil {
					return fmt.Sprintf("[%d..∞]", min)
				}
				return fmt.Sprintf("[%d..%d]", min, *max)
			},
			FormatGrammarLabel: func(label syntaxa.GrammarLabel) string {
				return "(" + string(label) + ")"
			},
		},
	)

	grammarPackage := compiler.parser.GetGrammarPackage()
	getAnalysis := func() *syntaxa.GrammarAnalysis[LangSpecLexerTokenType] { return lowering.GetAnalysis(grammarPackage) }
	grammarPackageDump := grammarPackage.DebugDump(
		syntaxa.GrammarPackageDebugFormatter[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, LangSpecLexerState]{
			FormatToken: LangSpecLexerTokenType.String,
		},
		getAnalysis,
	)

	var cfgDump string
	if cfg, _, _ := lowering.ToPatternGrammar(grammarPackage.Root, grammarPackage.AdditionalRules, grammarPackage.Grammars); cfg != nil {
		cfgDump = cfg.DebugDump(
			pattern.NewContextaCleanFormatter[LangSpecLexerTokenType, struct{}](LangSpecLexerTokenType.String),
		)
	}

	RenderGrammarDumps(compiler.diagnosticWriter, grammarDump, grammarPackageDump, cfgDump)
}

/*
LangSpecCompilerDebugResult writes parse trace and/or LST dump for result per config.

When config.DebugParseTrace is true, event-level trace output requires parse trace collection during LangSpecCompilerCompile (see LangSpecCompilerConfiguration.WithCollectParseTraceForLspec). Otherwise RenderParseTrace reports no trace data.
*/
func LangSpecCompilerDebugResult(compiler *LangSpecCompiler, result *LangSpecCompileResult, config *CompilerDebugConfig) {
	if config.DebugParseTrace {
		RenderParseTrace(compiler.diagnosticWriter, result.Trace, func(t LangSpecLexerTokenType) string { return t.String() })
	}

	if config.DebugLST {
		lstDump := result.RootNode.DebugDump(
			syntaxa.LSTDebugFormatter[
				rune,
				LangSpecLexerTokenType,
				LangSpecLexerTokenRole,
				LangSpecParserNodeKind,
			]{
				FormatKind: func(k LangSpecParserNodeKind) string {
					return k.String()
				},

				FormatToken: func(l lexarch.Lexeme[
					rune,
					LangSpecLexerTokenType,
					LangSpecLexerTokenRole,
				]) string {
					return string(l.Raw)
				},

				FormatAttribute: func(k string, v any) string {
					return fmt.Sprintf("%s=%v", k, v)
				},

				/* ───── visual toggles ───── */

				ShowTokens:     true,
				ShowAttributes: true,

				ShowByteSpan: true,
				ShowLineSpan: true,

				ShowNodeID:   true,
				ShowRevision: false,

				SlotPrefix: "@",

				/* colors disabled for now */
				ColorKind:      nil,
				ColorToken:     nil,
				ColorSpan:      nil,
				ColorAttribute: nil,
			},
		)

		RenderLSTDump(compiler.diagnosticWriter, lstDump)
	}
}

/*
LangSpecCompilerScopeMap returns the token-to-scope map from the compiler's language spec.
Used by editor integrations for syntax highlighting.
*/
func LangSpecCompilerScopeMap(compiler *LangSpecCompiler) map[LangSpecLexerTokenType]string {
	return dslspec.LanguageSpecScopeMap(compiler.languageSpec)
}

/*
LangSpecCompilerLexingRuleSet returns the lexing ruleset used by the DSL compiler.
Used by editor integrations to build syntax highlighting IR without depending on parser internals.
*/
func LangSpecCompilerLexingRuleSet(compiler *LangSpecCompiler) *lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole] {
	return compiler.lexingRuleSet
}

/*
LangSpecCompilerProgramRule returns the program rule used by the DSL compiler.
Used by editor integrations that need the rule or its grammar.
*/
func LangSpecCompilerProgramRule(compiler *LangSpecCompiler) Rule {
	return compiler.programRule
}

/*
LangSpecCompilerGrammarPackage returns the grammar package used by the DSL parser.
Use for editor IR (e.g. syntax highlighting) or debug dumps.
*/
func LangSpecCompilerGrammarPackage(compiler *LangSpecCompiler) *syntaxa.GrammarPackage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, LangSpecLexerState] {
	return compiler.parser.GetGrammarPackage()
}
