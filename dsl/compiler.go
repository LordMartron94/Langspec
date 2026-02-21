package dsl

import (
	"fmt"
	"foundation/system"
	"langspec"
	"langspec/validation"
	"lexarch"
	"memarch"
	"syntaxa"
	"syntaxa/rule"
)

type ValidationCode string

const (
	VALIDATION_DECLARATION_ALREADY_SEEN    ValidationCode = "V_D001"
	VALIDATION_DECLARATION_NOT_PRESENT     ValidationCode = "V_D002"
	VALIDATION_DECLARATION_DUPLICATE_ENTRY ValidationCode = "V_D003"
)

func (v ValidationCode) String() string {
	return string(v)
}

// --------------------------------------------------------------- TYPE ALIASES

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
	parser          *langspec.LangParser[rune, LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
	validatorConfig *validation.ASTValidatorConfiguration[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

	sessionCache *langspec.LangParserSession[rune]
}

/* LangSpecCompilerCreate constructs a compiler instance. */
func LangSpecCompilerCreate(compilerConfig *LangSpecCompilerConfiguration) *LangSpecCompiler {
	spec := buildLangSpecDSLSpec()
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
		parser:          parser,
		validatorConfig: validationConfig,
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

	// fmt.Println("\n===== DFA DEBUG DUMP =====")
	// fmt.Println(dfaDUMP)
	// fmt.Println("=========================")

	contentRune, _ := system.FileReadAllRunes(sourceFile)
	// fmt.Printf("DEBUG: rune content (escaped):\n%q\n", string(contentRune))
	// fmt.Println("DEBUG: rune stream:")
	// for i, r := range contentRune {
	// 	fmt.Printf("[%04d] rune=%q  codepoint=U+%04X\n", i, r, r)
	// }

	session := getSession(compiler, sourceFile)

	// lexemes, err := langspec.LangParserLexFile(compiler.parser, session)
	// if err != nil {
	// 	return fmt.Errorf("lexing error: %w", err)
	// }

	// for i, lexeme := range lexemes {
	// 	debug := lexeme.DebugString(
	// 		lexarch.RuneFormatterDefault(),
	// 		func(lsltt LangSpecLexerTokenType) string {
	// 			return lsltt.String()
	// 		},
	// 		func(lsltr LangSpecLexerTokenRole) string {
	// 			return lsltr.String()
	// 		},
	// 	)

	// 	fmt.Printf("%05d) %s\n", i, debug)
	// }

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

// --------------------------------------------------------------- PRIVATE HELPERS

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
