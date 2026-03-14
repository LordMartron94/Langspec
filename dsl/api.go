package dsl

import (
	"autarch/pattern"
	"fmt"
	"foundation/system"
	"io"
	"langspec"
	"langspec/validation"
	"lexarch"
	"memarch"
	"syntaxa"
	"syntaxa/lowering"
	"syntaxa/rule"
)

// --------------------------------------------------------------- TYPE ALIASES

type LexingRuleset = lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]

type RuleBuilder = rule.RuleBuilder[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Rule = rule.Rule[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Result = rule.Result[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationStage = validation.LSTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationStageCtx = validation.LSTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
type NodeFinalizationCtx = syntaxa.FinalizationCtx[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type Node = syntaxa.SyntaxaLSTNode[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

func AttributeAs[TAttribute any](node *Node, attributeName string) (TAttribute, bool) {
	return syntaxa.AttributeAs[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, TAttribute](
		node, attributeName,
	)
}

// --------------------------------------------------------------- CONFIGURATION

/* LangSpecCompilerConfiguration encapsulates the configuration for the langspec compiler. */
type LangSpecCompilerConfiguration struct {
	scratchAllocationFunction memarch.AllocationFn
	stageReporter             validation.LSTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	diagnosticSink            *LangSpecDiagnosticSink
}

/*
LangSpecCompilerConfigurationCreate creates an instance of the compiler configuration.

Stage reporter is optional. DiagnosticSink is optional; when set, compilation diagnostics
(syntax errors, trace, validation, LST dump) are written to the sink's Writer.
*/
func LangSpecCompilerConfigurationCreate(
	scratchAllocationFunction memarch.AllocationFn,
	stageReporter validation.LSTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
) *LangSpecCompilerConfiguration {
	return &LangSpecCompilerConfiguration{
		scratchAllocationFunction: scratchAllocationFunction,
		stageReporter:             stageReporter,
		diagnosticSink:            nil,
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

	CompiledGrammarPackage GrammarPackage

	EOFToken string
}

/*
LangSpecCompiler compiles a .lspec file into the LangSpec configuration needed by the LangParser.
*/
type LangSpecCompiler struct {
	config *LangSpecCompilerConfiguration

	languageSpec LanguageSpec

	parser          *langspec.LangParser[rune, LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	validatorConfig *validation.LSTValidatorConfiguration[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

	lexingRuleSet *lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]
	programRule   Rule

	sessionCache *langspec.LangParserSession[rune]

	diagnosticWriter io.Writer
}

/* LangSpecCompilerCreate constructs a compiler instance. */
func LangSpecCompilerCreate(compilerConfig *LangSpecCompilerConfiguration) *LangSpecCompiler {
	spec, dslSpec, ruleset, programRule := buildLangSpecDSLSpec()

	langParserConfig := langspec.LangParserConfigurationCreate(
		dslSpec,
		compilerConfig.scratchAllocationFunction,
	)
	parser := langspec.LangParserCreate(langParserConfig)

	validationConfig := validation.LSTValidatorConfigurationCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]()
	validationConfig = validationConfig.WithStageReporter(compilerConfig.stageReporter).WithStages(
		getValidationStages()...,
	)

	var w io.Writer
	if compilerConfig.diagnosticSink != nil && compilerConfig.diagnosticSink.Writer != nil {
		w = compilerConfig.diagnosticSink.Writer
	}

	return &LangSpecCompiler{
		config:           compilerConfig,
		languageSpec:     spec,
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
*/
func LangSpecCompilerCompile(
	compiler *LangSpecCompiler,
	sourceFile string,
) (*LangSpecCompileResult, error) {
	if !system.PathHasExt(sourceFile, ".lspec") {
		return nil, fmt.Errorf("file is not a .lspec file: %s", sourceFile)
	}

	contentRune, _ := system.FileReadAllRunes(sourceFile)
	session := getSession(compiler, sourceFile)

	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(
		compiler.parser,
		session,
	)

	result := &LangSpecCompileResult{
		RootNode:     rootNode,
		Trace:        trace,
		SyntaxErrors: syntaxErrors,
	}

	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		RenderSyntaxErrorsWithContext(compiler.diagnosticWriter, contentRune, syntaxErrors, lexarch.ColumnAdvanceRune(4))
		err = fmt.Errorf(
			"langspec parse failed with %d syntax errors",
			len(syntaxErrors.Errors),
		)
	}

	// Validation entries
	if !syntaxErrors.HasErrors() {
		validationEntries, validationErr := validation.LSTValidatorRun(compiler.validatorConfig, rootNode)
		if validationErr != nil {
			err = validationErr
		}

		result.ValidationEntries = validationEntries

		if validationEntries != nil && len(validationEntries.Results) > 0 {
			renderValidationEntries(compiler.diagnosticWriter, contentRune, validationEntries, lexarch.ColumnAdvanceRune(4))

			errorAmount := 0
			for _, stage := range validationEntries.Results {
				for _, entry := range stage.Entries {
					if entry.Severity > validation.VALIDATION_SEVERITY_INFO {
						errorAmount++
					}
				}
			}

			if errorAmount > 0 {
				err = fmt.Errorf("parsing failed with %d validation errors", errorAmount)
			}
		}
	}

	if err == nil {
		compiled := compileTree(compiler, result.RootNode)
		result.LanguageName = compiled.dslName
		result.LanguageVersion = compiled.dslVersion

		result.CompiledLexerSpec = compiled.lexerSpec
		result.CompiledParserSpec = compiled.parserSpec
		result.CompiledGrammarPackage = compiled.grammarPackage

		result.EOFToken = compiled.eofToken
	}

	return result, err
}

type CompilerDebugConfig struct {
	DebugParseTrace bool
	DebugLST        bool
}

func LangSpecCompilerDebugLexemes(compiler *LangSpecCompiler, sourceFile string) error {
	session := getSession(compiler, sourceFile)

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
			pattern.NewContextaCleanFormatter(LangSpecLexerTokenType.String),
		)
	}

	RenderGrammarDumps(compiler.diagnosticWriter, grammarDump, grammarPackageDump, cfgDump)
}

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
	return LanguageSpecScopeMap(compiler.languageSpec)
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
