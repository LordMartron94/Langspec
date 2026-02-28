package dsl

import (
	"fmt"
	"foundation/system"
	"io"
	"langspec"
	"langspec/validation"
	"lexarch"
	"memarch"
	"syntaxa"
	"syntaxa/rule"
)

// --------------------------------------------------------------- TYPE ALIASES

type LexingRuleset = lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]

type RuleBuilder = rule.RuleBuilder[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Rule = rule.Rule[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]
type Result = rule.Result[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationStage = validation.ASTValidationStage[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type ValidationStageCtx = validation.ASTValidationStageContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
type NodeFinalizationCtx = syntaxa.FinalizationCtx[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

type Node = syntaxa.SyntaxaASTNode[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

func AttributeAs[TAttribute any](node *Node, attributeName string) (TAttribute, bool) {
	return syntaxa.AttributeAs[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind, TAttribute](
		node, attributeName,
	)
}

// --------------------------------------------------------------- CONFIGURATION

/* LangSpecCompilerConfiguration encapsulates the configuration for the langspec compiler. */
type LangSpecCompilerConfiguration struct {
	scratchAllocationFunction memarch.AllocationFn
	stageReporter             validation.ASTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	diagnosticSink            *LangSpecDiagnosticSink
}

/*
LangSpecCompilerConfigurationCreate creates an instance of the compiler configuration.

Stage reporter is optional. DiagnosticSink is optional; when set, compilation diagnostics
(syntax errors, trace, validation, AST dump) are written to the sink's Writer.
*/
func LangSpecCompilerConfigurationCreate(
	scratchAllocationFunction memarch.AllocationFn,
	stageReporter validation.ASTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
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
LangSpecCompileResult holds the result of compiling a .lspec file: the parsed AST,
parse trace, syntax errors (if any), and validation entries (when validation was run).
Callers can inspect the result without parsing stdout.
*/
type LangSpecCompileResult struct {
	RootNode           *Node
	Trace              *syntaxa.ParseTrace[LangSpecLexerTokenType]
	SyntaxErrors       *syntaxa.SyntaxErrors[rune]
	ValidationEntries  *validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
}

/*
LangSpecCompiler compiles a .lspec file into the LangSpec configuration needed by the LangParser.
*/
type LangSpecCompiler struct {
	config *LangSpecCompilerConfiguration

	languageSpec LanguageSpec

	parser          *langspec.LangParser[rune, LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	validatorConfig *validation.ASTValidatorConfiguration[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

	lexingRuleSet *lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]
	programRule   Rule

	sessionCache *langspec.LangParserSession[rune]
}

/* LangSpecCompilerCreate constructs a compiler instance. */
func LangSpecCompilerCreate(compilerConfig *LangSpecCompilerConfiguration) *LangSpecCompiler {
	spec, dslSpec, ruleset, programRule := buildLangSpecDSLSpec()

	langParserConfig := langspec.LangParserConfigurationCreate(
		dslSpec,
		compilerConfig.scratchAllocationFunction,
	)
	parser := langspec.LangParserCreate(langParserConfig)

	validationConfig := validation.ASTValidatorConfigurationCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]()
	validationConfig = validationConfig.WithStageReporter(compilerConfig.stageReporter).WithStages(
		getValidationStages()...,
	)

	return &LangSpecCompiler{
		config:          compilerConfig,
		languageSpec:    spec,
		parser:          parser,
		validatorConfig: validationConfig,
		lexingRuleSet:   ruleset,
		programRule:     programRule,
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
with the parsed AST, trace, syntax errors, and validation entries (if validation ran).
Diagnostic output is written only when config's DiagnosticSink is set.
*/
func LangSpecCompilerCompile(
	compiler *LangSpecCompiler,
	sourceFile string,
) (*LangSpecCompileResult, error) {

	if !system.PathHasExt(sourceFile, ".lspec") {
		return nil, fmt.Errorf("file is not a .lspec file: %s", sourceFile)
	}

	var w io.Writer
	if compiler.config.diagnosticSink != nil && compiler.config.diagnosticSink.Writer != nil {
		w = compiler.config.diagnosticSink.Writer
	}

	contentRune, _ := system.FileReadAllRunes(sourceFile)

	grammarDump := compiler.programRule.GetGrammar().DebugDump(
		syntaxa.GrammarDebugFormatter[LangSpecLexerTokenType]{
			FormatKind:  syntaxa.GrammarKind.String,
			FormatToken: LangSpecLexerTokenType.String,
			FormatRange: func(min int, max *int) string {
				if max == nil {
					return fmt.Sprintf("[%d..∞]", min)
				}
				return fmt.Sprintf("[%d..%d]", min, *max)
			},
			FormatGrammarID: func(id syntaxa.GrammarID) string {
				return "(" + string(id) + ")"
			},
		},
	)

	grammarPackage := compiler.programRule.GetGrammar().ProducePackage("LangSpec DSL", "0.0.0")
	grammarPackageDump := grammarPackage.DebugDump(syntaxa.GrammarPackageDebugFormatter[LangSpecLexerTokenType]{
		FormatToken: LangSpecLexerTokenType.String,
	})

	renderGrammarDumps(w, grammarDump, grammarPackageDump)

	session := getSession(compiler, sourceFile)

	lexemes, err := langspec.LangParserLexFile(compiler.parser, session)
	if err != nil {
		return nil, fmt.Errorf("lexing error: %w", err)
	}

	renderLexemes(w, lexemes,
		func(t LangSpecLexerTokenType) string { return t.String() },
		func(r LangSpecLexerTokenRole) string { return r.String() },
	)

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
		renderSyntaxErrorsWithContext(w, contentRune, syntaxErrors)
		err = fmt.Errorf(
			"langspec parse failed with %d syntax errors",
			len(syntaxErrors.Errors),
		)
	}

	renderParseTrace(w, trace, func(t LangSpecLexerTokenType) string { return t.String() })

	// Validation entries
	if !syntaxErrors.HasErrors() {
		validationEntries, validationErr := validation.ASTValidatorRun(compiler.validatorConfig, rootNode)
		if validationErr != nil {
			err = validationErr
		}

		result.ValidationEntries = validationEntries

		if validationEntries != nil && len(validationEntries.Results) > 0 {
			renderValidationEntries(w, validationEntries)

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

	// AST dump (visual ground truth)
	dump := rootNode.DebugDump(
		syntaxa.ASTDebugFormatter[
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

	renderASTDump(w, dump)

	return result, err
}

// --------------------------------------------------------------- PRIVATE HELPERS

/*
LangSpecCompilerScopeMap returns the token-to-scope map from the compiler's language spec.
Used by editor integrations for syntax highlighting.
*/
func LangSpecCompilerScopeMap(compiler *LangSpecCompiler) map[LangSpecLexerTokenType]string {
	return LanguageSpecScopeMap(compiler.languageSpec)
}

/*
LangSpecCompilerHeaderSpec returns the header nest steps from the compiler's language spec.
Used by editor integrations for Sublime nest overrides.
*/
func LangSpecCompilerHeaderSpec(compiler *LangSpecCompiler) []DSLHeaderNestStep {
	return compiler.languageSpec.Headers
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
Used by editor integrations to produce the grammar package for syntax highlighting.
*/
func LangSpecCompilerProgramRule(compiler *LangSpecCompiler) Rule {
	return compiler.programRule
}

var runeFormatter = lexarch.RuneFormatterDefault()
