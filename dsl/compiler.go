package dsl

import (
	"autarch/pattern"
	"fmt"
	"foundation/system"
	"langspec"
	"langspec/validation"
	"lexarch"
	"memarch"
	"strings"
	"syntaxa"
	"syntaxa/rule"
)

// --------------------------------------------------------------- TYPES

type LangSpecLexerState uint8

const (
	LANG_SPEC_LEXER_STATE_DEFAULT LangSpecLexerState = iota + 1
)

type LangSpecLexerTokenType uint32

const (
	// Core
	LANG_SPEC_LEXER_EOF_TOKEN LangSpecLexerTokenType = iota + 1

	LANG_SPEC_LEXER_WHITESPACE

	// Header structure
	LANG_SPEC_LEXER_HEADER_DASHES
	LANG_SPEC_LEXER_HEADER_SEPARATOR

	LANG_SPEC_LEXER_STRING_LITERAL
	LANG_SPEC_LEXER_VERSION

	LANG_SPEC_LEXER_KW_LSPEC

	LANG_SPEC_LEXER_KW_DECLARE
	LANG_SPEC_LEXER_KW_LEXER_STATES
	LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES

	LANG_SPEC_LEXER_BRACKET_OPEN
	LANG_SPEC_LEXER_BRACKET_CLOSE
	LANG_SPEC_LEXER_SEMICOLON
	LANG_SPEC_LEXER_COMMA
)

func (l LangSpecLexerTokenType) String() string {
	switch l {
	case LANG_SPEC_LEXER_EOF_TOKEN:
		return "EOF"
	case LANG_SPEC_LEXER_WHITESPACE:
		return "WHITESPACE"
	case LANG_SPEC_LEXER_HEADER_DASHES:
		return "DASHES"
	case LANG_SPEC_LEXER_HEADER_SEPARATOR:
		return "HEADER SEPARATOR"
	case LANG_SPEC_LEXER_STRING_LITERAL:
		return "STRING LITERAL"
	case LANG_SPEC_LEXER_VERSION:
		return "VERSION"
	case LANG_SPEC_LEXER_KW_DECLARE:
		return "DECLARE KW"
	case LANG_SPEC_LEXER_KW_LEXER_STATES:
		return "LEXER STATES KW"
	case LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES:
		return "TOKEN TYPES KW"
	case LANG_SPEC_LEXER_BRACKET_OPEN:
		return "BRACKET OPEN"
	case LANG_SPEC_LEXER_BRACKET_CLOSE:
		return "BRACKET CLOSE"
	case LANG_SPEC_LEXER_SEMICOLON:
		return "SEMICOLON"
	case LANG_SPEC_LEXER_KW_LSPEC:
		return "LSPEC KW"
	case LANG_SPEC_LEXER_COMMA:
		return "COMMA"
	default:
		return "UNKNOWN TOKEN TYPE"
	}
}

type LangSpecLexerTokenRole uint8

const (
	LANG_SPEC_STRUCTURAL_ROLE LangSpecLexerTokenRole = iota + 1
	LANG_SPEC_IGNORED_ROLE
	LANG_SPEC_WHITESPACE_ROLE
)

func (l LangSpecLexerTokenRole) String() string {
	switch l {
	case LANG_SPEC_STRUCTURAL_ROLE:
		return "Structural"
	case LANG_SPEC_IGNORED_ROLE:
		return "IGNORED"
	case LANG_SPEC_WHITESPACE_ROLE:
		return "WHITESPACE"
	default:
		return "UNKNOWN TOKEN ROLE"
	}
}

type LangSpecParserNodeKind uint32

const (
	LANG_SPEC_ERROR_NODE LangSpecParserNodeKind = iota + 1
	LANG_SPEC_PROGRAM_NODE

	LANG_SPEC_HEADER_NODE
	LANG_SPEC_DECLARATION_BLOCK_NODE

	LANG_SPEC_DSL_NAME_NODE
	LANG_SPEC_VERSION_NODE
	LANG_SPEC_LSPEC_NAME_NODE

	LANG_SPEC_DECLARE_IDENTIFIER_NODE
	LANG_SPEC_IDENTIFIER_NODE
	LANG_SPEC_LIST_NODE

	LANG_SPEC_BODY_NODE
	LANG_SPEC_DECLARATION_BLOCKS_NODE
)

func (k LangSpecParserNodeKind) String() string {
	switch k {
	case LANG_SPEC_ERROR_NODE:
		return "ERROR"
	case LANG_SPEC_PROGRAM_NODE:
		return "PROGRAM"
	case LANG_SPEC_HEADER_NODE:
		return "HEADER"
	case LANG_SPEC_DSL_NAME_NODE:
		return "DSL NAME"
	case LANG_SPEC_VERSION_NODE:
		return "VERSION"
	case LANG_SPEC_LSPEC_NAME_NODE:
		return "LSPEC NAME"
	case LANG_SPEC_DECLARATION_BLOCK_NODE:
		return "DECLARE BLOCK"
	case LANG_SPEC_DECLARE_IDENTIFIER_NODE:
		return "DECLARE IDENTIFIER"
	case LANG_SPEC_IDENTIFIER_NODE:
		return "IDENTIFIER"
	case LANG_SPEC_LIST_NODE:
		return "LIST"
	case LANG_SPEC_BODY_NODE:
		return "BODY"
	case LANG_SPEC_DECLARATION_BLOCKS_NODE:
		return "DECLARATION BLOCKS"
	default:
		return "UNKNOWN NODE KIND"
	}
}

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

func buildLangSpecDSLSpec() *langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind] {
	lexerSpec := buildLangSpecDSLLexerSpec()
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	parserSpec := buildLangSpecDSLParserSpec()

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return dslSpec
}

func buildLangSpecDSLLexerSpec() *langspec.LexerSpec[
	rune,
	LangSpecLexerTokenType,
	LangSpecLexerTokenRole,
	LangSpecLexerState,
] {
	lexerSpec := langspec.LexerSpecCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole](
		LANG_SPEC_LEXER_EOF_TOKEN,
		LANG_SPEC_LEXER_STATE_DEFAULT,
		lexarch.NewlineDetectorRune(),
		lexarch.ColumnAdvanceRune(4),
		runeFormatter,
		lexarch.LexarchRuneSuccessorFn(),
		func(token LangSpecLexerTokenType) string {
			return token.String()
		},
	)

	defaultRuleset := lexarch.LexingRulesetCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole,
	](
		lexarch.TokenResolutionStepLongestThenPriority[LangSpecLexerTokenType],
	)

	// ------------------------------------------------------------
	// Whitespace (ignored)
	// ------------------------------------------------------------

	ws := pattern.AnyOf(
		pattern.Literal(' '),
		pattern.Literal('\t'),
		pattern.Literal('\n'),
	).Plus()

	defaultRuleset.WithRule(ws, LANG_SPEC_LEXER_WHITESPACE, LANG_SPEC_WHITESPACE_ROLE)

	// ------------------------------------------------------------
	// Header structure
	// ------------------------------------------------------------

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("---"),
		LANG_SPEC_LEXER_HEADER_DASHES,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal('|'),
		LANG_SPEC_LEXER_HEADER_SEPARATOR,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal(','),
		LANG_SPEC_LEXER_COMMA,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal(';'),
		LANG_SPEC_LEXER_SEMICOLON,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal('{'),
		LANG_SPEC_LEXER_BRACKET_OPEN,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal('}'),
		LANG_SPEC_LEXER_BRACKET_CLOSE,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	// ------------------------------------------------------------
	// Language name: "TEST LANGUAGE"
	// ------------------------------------------------------------

	notQuote := pattern.Class(
		pattern.Range(0, '"'-1),
		pattern.Range('"'+1, rune(0x10FFFF)),
	)

	quoted := pattern.Sequence(
		pattern.Literal('"'),
		notQuote.Star(),
		pattern.Literal('"'),
	)

	defaultRuleset.WithRulePriority(
		quoted,
		LANG_SPEC_LEXER_STRING_LITERAL,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	// ------------------------------------------------------------
	// Versions: v1.0.0
	// ------------------------------------------------------------

	digits := pattern.Digit.Plus()

	version := pattern.Sequence(
		pattern.Literal('v'),
		digits,
		pattern.Literal('.'),
		digits,
		pattern.Literal('.'),
		digits,
	)

	defaultRuleset.WithRulePriority(
		version,
		LANG_SPEC_LEXER_VERSION,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	// ------------------------------------------------------------
	// DSL name: lspec
	// ------------------------------------------------------------

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("lspec"),
		LANG_SPEC_LEXER_KW_LSPEC,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("declare"),
		LANG_SPEC_LEXER_KW_DECLARE,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("LexerStates"),
		LANG_SPEC_LEXER_KW_LEXER_STATES,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("LexerTokenTypes"),
		LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	// ------------------------------------------------------------

	lexerSpec.WithRuleset(
		LANG_SPEC_LEXER_STATE_DEFAULT,
		*defaultRuleset,
	)

	return lexerSpec
}

const (
	ATTRIBUTE_LITERAL_STRING_FORMATTED = "formattedString"
	ATTRIBUTE_RULE_NAME                = "ruleName"
)

func buildLangSpecDSLParserSpec() *langspec.ParserSpec[
	rune,
	LangSpecLexerTokenType,
	LangSpecLexerTokenRole,
	LangSpecLexerState,
	LangSpecParserNodeKind,
] {
	finalizationPostProcessor := func(node *Node, finalizationCTX *NodeFinalizationCtx, ruleIdentity syntaxa.RuleIdentity) {
		// fmt.Printf("Post processing node created by: %s\n", ruleIdentity.RuleName)
		finalizationCTX.SetAttribute(node, ATTRIBUTE_RULE_NAME, ruleIdentity.RuleName)

		lexemes := node.Tokens()
		for _, lexeme := range lexemes {
			if lexeme.Token == LANG_SPEC_LEXER_STRING_LITERAL {
				raw := node.GetContent(" ")
				formatted := strings.Replace(raw, "\"", "", -1)
				finalizationCTX.SetAttribute(node, ATTRIBUTE_LITERAL_STRING_FORMATTED, formatted)
				break
			}
		}
	}

	ruleBuilder := rule.RuleBuilderCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind](
		LangSpecLexerTokenType.String,
	)

	parserSpec := langspec.ParserSpecCreate(
		LANG_SPEC_PROGRAM_NODE,
		LANG_SPEC_ERROR_NODE,
		parseProgram(ruleBuilder),
		true, // freeze AST after parse
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_IGNORED_ROLE,
	)
	parserSpec.WithPostProcessor(finalizationPostProcessor)

	return parserSpec
}

func parseProgram(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Root(
		"PROGRAM",
		LANG_SPEC_PROGRAM_NODE,
		false,
		parseHeader(ruleBuilder),
		parseBody(ruleBuilder),
		ruleBuilder.Token.ExpectVirtual("EOF", LANG_SPEC_LEXER_EOF_TOKEN),
	)
}

func parseHeader(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Sequence(
		"HEADER",
		LANG_SPEC_HEADER_NODE,
		ruleBuilder.Token.ExpectVirtual("HEADER START", LANG_SPEC_LEXER_HEADER_DASHES),
		ruleBuilder.Token.Expect("DSL NAME", LANG_SPEC_DSL_NAME_NODE, LANG_SPEC_LEXER_STRING_LITERAL),
		ruleBuilder.Token.Expect("DSL VERSION", LANG_SPEC_VERSION_NODE, LANG_SPEC_LEXER_VERSION),
		ruleBuilder.Token.ExpectVirtual("HEADER SEPARATOR", LANG_SPEC_LEXER_HEADER_SEPARATOR),
		ruleBuilder.Token.ExpectOneOf("LANGSPEC NAME", LANG_SPEC_LSPEC_NAME_NODE, LANG_SPEC_LEXER_STRING_LITERAL, LANG_SPEC_LEXER_KW_LSPEC),
		ruleBuilder.Token.Expect("LANGSPEC VERSION", LANG_SPEC_VERSION_NODE, LANG_SPEC_LEXER_VERSION),
		ruleBuilder.Token.ExpectVirtual("HEADER END", LANG_SPEC_LEXER_HEADER_DASHES),
	)
}

func parseBody(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.ZeroOrMore("DECLARATION BLOCKS", LANG_SPEC_DECLARATION_BLOCKS_NODE, parseDeclarationBlock(ruleBuilder))
}

func parseDeclarationBlock(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Block(
		"DECLARATION BLOCK",
		LANG_SPEC_DECLARATION_BLOCK_NODE,
		LANG_SPEC_LEXER_SEMICOLON,
		ruleBuilder.Token.ExpectVirtual("DECLARE KEYWORD", LANG_SPEC_LEXER_KW_DECLARE),
		ruleBuilder.Token.ExpectOneOf("DECLARE IDENTIFIER", LANG_SPEC_IDENTIFIER_NODE, LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES, LANG_SPEC_LEXER_KW_LEXER_STATES),
		ruleBuilder.Token.List(
			"DECLARE LIST",
			LANG_SPEC_LEXER_BRACKET_OPEN,
			LANG_SPEC_LEXER_STRING_LITERAL, LANG_SPEC_LEXER_COMMA,
			LANG_SPEC_LEXER_BRACKET_CLOSE,
			LANG_SPEC_LIST_NODE, LANG_SPEC_DECLARE_IDENTIFIER_NODE,
			true, // Allow empty list
			rule.TrailingOptional,
		),
		ruleBuilder.Token.ExpectVirtual("BLOCK CLOSE", LANG_SPEC_LEXER_SEMICOLON),
	)
}

func getValidationStages() []*ValidationStage {
	stages := []*ValidationStage{
		validation.ASTValidationStageCreate(
			"Declaration Block Validation",
			"Validates whether declaration blocks are correct in terms of presence an uniqueness.",
			0,
			func(ctx *ValidationStageCtx) {
				root := ctx.RootNode
				blocks := root.FindAllKind(LANG_SPEC_DECLARATION_BLOCK_NODE)

				required := []LangSpecLexerTokenType{
					LANG_SPEC_LEXER_KW_LEXER_STATES,
					LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES,
				}
				seenSet := map[LangSpecLexerTokenType]struct{}{}

				for _, block := range blocks {
					identifierNode := block.Children()[0]
					tokenType := identifierNode.Tokens()[0].Token

					if _, seen := seenSet[tokenType]; seen {
						ctx.ReportError(
							VALIDATION_DECLARATION_ALREADY_SEEN.String(),
							fmt.Sprintf("declaration node '%s' is not unique", identifierNode.GetContent(" ")),
							identifierNode,
						)
					} else {
						seenSet[tokenType] = struct{}{}
					}

					seenDeclarationsSet := map[string]struct{}{}
					identifiers := block.FindAllKind(LANG_SPEC_DECLARE_IDENTIFIER_NODE)
					for _, identifier := range identifiers {
						identifierValue, _ := AttributeAs[string](identifier, ATTRIBUTE_LITERAL_STRING_FORMATTED)
						if _, seen := seenDeclarationsSet[identifierValue]; seen {
							ctx.ReportError(
								VALIDATION_DECLARATION_DUPLICATE_ENTRY.String(),
								fmt.Sprintf("duplicate declaration entry '%s'", identifierValue),
								identifierNode,
							)
						} else {
							seenDeclarationsSet[identifierValue] = struct{}{}
						}
					}
				}

				for _, requiredTk := range required {
					if _, seen := seenSet[requiredTk]; !seen {
						ctx.ReportError(
							VALIDATION_DECLARATION_NOT_PRESENT.String(),
							fmt.Sprintf("declaration node '%s' is not present", requiredTk.String()),
							nil,
						)
					}
				}
			},
		),
	}

	return stages
}

func renderSyntaxErrorsWithContext(source []rune, errs *syntaxa.SyntaxErrors[rune]) {
	if len(errs.Errors) == 0 {
		return
	}

	lines := splitLinesRunes(source)

	fmt.Println("\n===== SYNTAX ERRORS =====")

	for _, e := range errs.Errors {
		printErrorHeader(e)

		// For now we only render nice underlines for single-line spans
		if e.StartLine != e.EndLine || e.StartLine <= 0 || !isValidLine(e.StartLine, len(lines)) {
			fmt.Println(" ── INVALID ERROR SPAN DETECTED ─────────────────────────────")
			fmt.Printf("  Message: %s\n", e.Message)
			fmt.Printf("  StartLine: %d  StartColumn: %d\n", e.StartLine, e.StartColumn)
			fmt.Printf("  EndLine:   %d  EndColumn:   %d\n", e.EndLine, e.EndColumn)
			fmt.Printf("  Total lines: %d\n", len(lines))

			var lineIdx int
			switch {
			case e.StartLine > 0 && e.StartLine <= len(lines):
				lineIdx = e.StartLine - 1
			case len(lines) > 0:
				lineIdx = len(lines) - 1
			default:
				fmt.Println("(no source available)")
				fmt.Println()
				continue
			}

			line := lines[lineIdx]

			fmt.Printf(" %4d | %s\n", lineIdx+1, string(line))
			fmt.Print("      | ")
			fmt.Println(renderSpan(line, e.StartColumn, e.EndColumn, 4))

			fmt.Println(" ──────────────────────────────────────────────────────────")
			continue
		}

		// Single-line span
		line := lines[e.StartLine-1]

		fmt.Printf(" %4d | %s\n", e.StartLine, string(line))
		fmt.Print("      | ")
		fmt.Println(renderSpan(line, e.StartColumn, e.EndColumn, 4))

		fmt.Println()
	}

	fmt.Println("========================")
}

func renderSpan(
	line []rune,
	startCol, endCol int,
	tabWidth int,
) string {

	if endCol < startCol {
		endCol = startCol
	}
	if startCol < 1 {
		startCol = 1
	}
	if endCol < 1 {
		endCol = 1
	}

	visualStart := calculateVisualOffset(line, startCol, tabWidth)
	visualEnd := calculateVisualOffset(line, endCol, tabWidth)

	var sb strings.Builder
	sb.WriteString(strings.Repeat(" ", visualStart))

	spanWidth := visualEnd - visualStart
	if spanWidth <= 0 {
		sb.WriteString("^")
	} else if spanWidth == 1 {
		sb.WriteString("^")
	} else {
		sb.WriteString("^")
		sb.WriteString(strings.Repeat("~", spanWidth-1))
	}

	return sb.String()
}

func calculateVisualOffset(line []rune, targetCol int, tabWidth int) int {
	visualPos := 0

	for col := 1; col < targetCol; col++ {
		runeIdx := col - 1

		if runeIdx < len(line) {
			visualPos += getCharacterVisualWidth(line[runeIdx], visualPos, tabWidth)
		} else {
			visualPos++
		}
	}

	return visualPos
}

func getCharacterVisualWidth(r rune, currentVisualPos int, tabWidth int) int {
	if r == '\t' {
		return tabWidth - (currentVisualPos % tabWidth)
	}
	return 1
}

func printErrorHeader(e syntaxa.SyntaxError[rune]) {
	typeStr := "syntax"
	if e.ProducedByLexer {
		typeStr = "lexer"
	}
	fmt.Printf("[%s] (rule=%s) %s at %d:%d\n", typeStr, e.Rule, e.Message, e.StartLine, e.StartColumn)
}

func isValidLine(lineNum, totalLines int) bool {
	return lineNum > 0 && lineNum <= totalLines
}

func splitLinesRunes(runes []rune) [][]rune {
	var lines [][]rune
	start := 0

	for i, r := range runes {
		if r == '\n' {
			lines = append(lines, runes[start:i])
			start = i + 1
		}
	}

	if start < len(runes) {
		lines = append(lines, runes[start:])
	}

	return lines
}

func renderParseTrace[TToken any](
	trace *syntaxa.ParseTrace[TToken],
	formatToken func(TToken) string,
) {
	if trace == nil || len(trace.Events) == 0 {
		fmt.Println("\n===== PARSE TRACE =====")
		fmt.Println("  (no trace data)")
		fmt.Println("======================")
		return
	}

	fmt.Println("\n===== PARSE TRACE =====")

	for i, ev := range trace.Events {
		fmt.Printf(
			"%04d | cur=%d | raw=%s | logical=%s | ok=%v | cons=%v | node=%v \n",
			i,
			ev.Cursor,
			formatToken(ev.RawToken),
			formatToken(ev.LogicalToken),
			ev.RuleSucceeded,
			ev.Consumed,
			ev.NodeReturned,
		)
	}

	fmt.Println("======================")
}
