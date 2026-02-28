package dsl

import (
	"autarch/pattern"
	"fmt"
	"foundation/system"
	"langspec"
	"langspec/editor"
	"langspec/editor/sublime"
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
}

/*
LangSpecCompilerConfigurationCreate creates an instance of the compiler configuration.

Stage reporter is optional.
*/
func LangSpecCompilerConfigurationCreate(
	scratchAllocationFunction memarch.AllocationFn,
	stageReporter validation.ASTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
) *LangSpecCompilerConfiguration {
	return &LangSpecCompilerConfiguration{
		scratchAllocationFunction: scratchAllocationFunction,
		stageReporter:             stageReporter,
	}
}

// --------------------------------------------------------------- COMPILER

/*
LangSpecCompiler compiles a .lspec file into the LangSpec configuration needed by the LangParser.
*/
type LangSpecCompiler struct {
	config *LangSpecCompilerConfiguration

	parser          *langspec.LangParser[rune, LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	validatorConfig *validation.ASTValidatorConfiguration[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

	lexingRuleSet *lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]
	programRule   Rule

	sessionCache *langspec.LangParserSession[rune]
}

/* LangSpecCompilerCreate constructs a compiler instance. */
func LangSpecCompilerCreate(compilerConfig *LangSpecCompilerConfiguration) *LangSpecCompiler {
	spec, ruleset, programRule := buildLangSpecDSLSpec()

	langParserConfig := langspec.LangParserConfigurationCreate(
		spec,
		compilerConfig.scratchAllocationFunction,
	)
	parser := langspec.LangParserCreate(langParserConfig)

	validationConfig := validation.ASTValidatorConfigurationCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]()
	validationConfig = validationConfig.WithStageReporter(compilerConfig.stageReporter).WithStages(
		getValidationStages()...,
	)

	return &LangSpecCompiler{
		config:          compilerConfig,
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
LangSpecCompilerCompile compiles a .lspec file into a LangSpec specification.
*/
func LangSpecCompilerCompile(
	compiler *LangSpecCompiler,
	sourceFile string,
) error {

	if !system.PathHasExt(sourceFile, ".lspec") {
		return fmt.Errorf("file is not a .lspec file: %s", sourceFile)
	}

	// dfaDUMP := compiler.parser.DebugDumpAllLexerDFAs()

	// fmt.Println("\n===== LEXER DFA DEBUG DUMP =====")
	// fmt.Println(dfaDUMP)
	// fmt.Println("=========================")

	contentRune, _ := system.FileReadAllRunes(sourceFile)
	// fmt.Printf("DEBUG: rune content (escaped):\n%q\n", string(contentRune))
	// fmt.Println("DEBUG: rune stream:")
	// for i, r := range contentRune {
	// 	fmt.Printf("[%04d] rune=%q  codepoint=U+%04X\n", i, r, r)
	// }

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

	fmt.Println("\n===== GRAMMAR DEBUG DUMP =====")
	fmt.Println(grammarDump)
	fmt.Println("=========================")

	grammarPackage := compiler.programRule.GetGrammar().ProducePackage("LangSpec DSL", "0.0.0")
	grammarPackageDump := grammarPackage.DebugDump(syntaxa.GrammarPackageDebugFormatter[LangSpecLexerTokenType]{
		FormatToken: LangSpecLexerTokenType.String,
	})

	fmt.Println("\n===== GRAMMAR PACKAGE DEBUG DUMP =====")
	fmt.Println(grammarPackageDump)
	fmt.Println("=========================")

	session := getSession(compiler, sourceFile)

	lexemes, err := langspec.LangParserLexFile(compiler.parser, session)
	if err != nil {
		return fmt.Errorf("lexing error: %w", err)
	}

	for i, lexeme := range lexemes {
		debug := lexeme.DebugString(
			func(lsltt LangSpecLexerTokenType) string {
				return lsltt.String()
			},
			func(lsltr LangSpecLexerTokenRole) string {
				return lsltr.String()
			},
		)

		fmt.Printf("%05d) %s\n", i, debug)
	}

	// return nil

	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(
		compiler.parser,
		session,
	)

	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		renderSyntaxErrorsWithContext(
			contentRune,
			syntaxErrors,
		)
		err = fmt.Errorf(
			"langspec parse failed with %d syntax errors",
			len(syntaxErrors.Errors),
		)
	}

	renderParseTrace(
		trace,
		func(t LangSpecLexerTokenType) string {
			return t.String()
		},
	)

	// ============================================================
	// Validation entries
	// ============================================================

	if !syntaxErrors.HasErrors() {
		validationEntries, validationErr := validation.ASTValidatorRun(compiler.validatorConfig, rootNode)
		if validationErr != nil {
			err = validationErr
		}

		if validationEntries != nil && len(validationEntries.Results) > 0 {
			fmt.Println("\n===== VALIDATION =====")

			errorAmount := 0
			for _, stage := range validationEntries.Results {
				fmt.Printf("\n-- Stage: %s (order %d) --\n", stage.StageName, stage.Order)

				if len(stage.Entries) == 0 {
					fmt.Println("  ✔ no issues")
					continue
				}

				for _, entry := range stage.Entries {
					if entry.Severity > validation.VALIDATION_SEVERITY_INFO {
						errorAmount++
					}

					fmt.Printf(
						"  [%v] %s — %s\n",
						entry.Severity,
						entry.Code,
						entry.Message,
					)
				}
			}

			fmt.Println("=======================")

			if errorAmount > 0 {
				err = fmt.Errorf("parsing failed with %d validation errors", errorAmount)
			}
		}
	}

	// ============================================================
	// AST dump (visual ground truth)
	// ============================================================

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

	fmt.Println("\n===== AST DEBUG DUMP =====")
	fmt.Println(dump)
	fmt.Println("=========================")

	return err
}

/*
LangSpecCompilerBuildSublimeSyntax generates a Sublime Text syntax definition file from the
compiler's grammar and lexer. It builds an editor IR via PushDownAutomatonIRConfiguration
with token overrides (using the editor's structural helpers for delimited regions and
match-with-capture) and a nest override for the HEADER production (using
BuildNestStateSequence), then writes the result to syntaxFile via the sublime package.
*/
func LangSpecCompilerBuildSublimeSyntax(compiler *LangSpecCompiler, syntaxFile string) error {
	editorIRConfig := editor.PushDownAutomatonIRConfigurationCreate[LangSpecLexerTokenType, LangSpecLexerTokenRole](
		getTokenScopes,
		LangSpecLexerTokenType.String,
		".lspec", // Scope Extension
	)

	editorIRConfig.AddPrototypeTokenRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_COMMENT_ROLE,
	)

	editorIRConfig.AddOverride(TokBlockComment, func(ctx *editor.TokenOverrideContext) (editor.StateRule, []editor.State) {
		openRegex, _ := pattern.LiteralString(factory, "/*").ToRegEx()
		closeRegex, _ := pattern.LiteralString(factory, "*/").ToRegEx()
		return editor.TokenOverrideDelimitedRegion(ctx,
			openRegex, closeRegex,
			"punctuation.definition.comment.begin",
			ctx.BaseScope,
			"punctuation.definition.comment.end",
		)
	})

	editorIRConfig.AddOverride(TokLineComment, func(ctx *editor.TokenOverrideContext) (editor.StateRule, []editor.State) {
		slashes := pattern.LiteralString(factory, "//").Capture()
		notTerminator := factory.NegatedClass(
			factory.Range('\n', '\n'),
			factory.Range('\r', '\r'),
		).Star().Capture()
		regex, _ := slashes.Then(notTerminator).ToRegEx()
		return editor.TokenOverrideMatchWithCapture(ctx,
			regex,
			"comment.line.double-slash",
			"punctuation.definition.comment",
		)
	})

	editorIRConfig.AddNestOverride("HEADER", func(ctx *editor.NestOverrideContext[LangSpecLexerTokenType]) (editor.StateID, []editor.State) {
		steps := []editor.NestStep[LangSpecLexerTokenType]{
			{
				LabelSuffix: "expect_name",
				MetaScope:   "meta.block.header",
				Rules: []editor.NestStepRule[LangSpecLexerTokenType]{
					{Token: TokStringLiteral, Scope: "entity.name.language", Action: editor.NestRuleActionPushNext},
				},
			},
			{
				LabelSuffix: "expect_version",
				Rules: []editor.NestStepRule[LangSpecLexerTokenType]{
					{Token: TokVersion, Scope: "constant.numeric.version", Action: editor.NestRuleActionPushNext},
				},
			},
			{
				LabelSuffix: "expect_tail",
				Rules: []editor.NestStepRule[LangSpecLexerTokenType]{
					{Token: TokDashes, Scope: "punctuation.definition.separator", Action: editor.NestRuleActionPop, PopCount: 3},
					{Token: TokHeaderSeparator, Scope: "punctuation.section.header", Action: editor.NestRuleActionMatch},
					{Token: TokStringLiteral, Scope: "string.quoted.double", Action: editor.NestRuleActionMatch},
					{Token: TokKWLSpec, Scope: "keyword.declaration.lspec", Action: editor.NestRuleActionMatch},
					{Token: TokVersion, Scope: "constant.numeric.version", Action: editor.NestRuleActionMatch},
				},
			},
		}
		return editor.BuildNestStateSequence(ctx, steps)
	})

	editorIR := editor.PushDownAutomatonIRCreate(
		editorIRConfig,
		compiler.lexingRuleSet,
		compiler.programRule.GetGrammar().ProducePackage("LangSpec DSL", "0.0.0"),
	)

	return sublime.SublimeTextGenerateSyntaxFile(
		editorIR,
		syntaxFile,
		[]string{".lspec"},
	)
}

// --------------------------------------------------------------- PRIVATE HELPERS

var tokenScopeMap = map[LangSpecLexerTokenType]string{
	TokEOF:               "meta.eof",
	TokWhitespace:        "punctuation.whitespace",
	TokDashes:            "punctuation.definition.separator",
	TokHeaderSeparator:   "punctuation.section.header",
	TokStringLiteral:     "string.quoted.double",
	TokVersion:           "constant.numeric.version",
	TokKWLSpec:           "keyword.declaration.lspec",
	TokKWDeclare:         "keyword.control.declare",
	TokKWLexerTokenTypes: "meta.type.builtin",
	TokBraceOpen:         "punctuation.section.braces.begin",
	TokBraceClose:        "punctuation.section.braces.end",
	TokSemicolon:         "punctuation.terminator.statement",
	TokComma:             "punctuation.separator.comma",
	TokLineComment:       "comment.line.double-slash",
	TokBlockComment:      "comment.block",
}

func getTokenScopes(token LangSpecLexerTokenType) string {
	if scope, ok := tokenScopeMap[token]; ok {
		return scope
	}
	return ""
}

var runeFormatter = lexarch.RuneFormatterDefault()

func getSession(compiler *LangSpecCompiler, sourceFile string) *langspec.LangParserSession[rune] {
	if compiler.sessionCache != nil {
		compiler.sessionCache.Reset(sourceFile, nil, false)
		return compiler.sessionCache
	} else {
		session := langspec.LangParserSessionCreate[rune](sourceFile, nil, false)
		compiler.sessionCache = session
		return session
	}
}

func buildLangSpecDSLSpec() (
	*langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	Rule,
) {
	lexerSpec, ruleset := buildLangSpecDSLLexerSpec()
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	parserSpec, programRule := buildLangSpecDSLParserSpec()

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return dslSpec, ruleset, programRule
}
