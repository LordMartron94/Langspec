package dsl

import (
	"autarch/pattern"
	"fmt"
	"foundation/system"
	"langspec"
	"lexarch"
	"memarch"
	"syntaxa"
	"syntaxa/rd"
)

// --------------------------------------------------------------- TYPES

type LangSpecLexerState uint8

const (
	LANG_SPEC_LEXER_STATE_DEFAULT LangSpecLexerState = iota + 1
)

type LangSpecLexerTokenType uint32

const (
	// Core
	LANG_SPEC_LEXER_ERROR_TOKEN LangSpecLexerTokenType = iota + 1
	LANG_SPEC_LEXER_EOF_TOKEN

	LANG_SPEC_LEXER_WHITESPACE

	// Header structure
	LANG_SPEC_LEXER_HEADER_DASHES    // ---
	LANG_SPEC_LEXER_HEADER_SEPARATOR // |

	// Language identity
	LANG_SPEC_LEXER_LANGUAGE_NAME // "TEST LANGUAGE"

	// Versions
	LANG_SPEC_LEXER_VERSION // v1.0.0 , v0.0.0

	// DSL identifier
	LANG_SPEC_LEXER_DSL_NAME // lspec
)

func (l LangSpecLexerTokenType) String() string {
	switch l {
	case LANG_SPEC_LEXER_ERROR_TOKEN:
		return "ERROR"
	case LANG_SPEC_LEXER_EOF_TOKEN:
		return "EOF"
	case LANG_SPEC_LEXER_WHITESPACE:
		return "WHITESPACE"
	case LANG_SPEC_LEXER_HEADER_DASHES:
		return "DASHES"
	case LANG_SPEC_LEXER_HEADER_SEPARATOR:
		return "HEADER SEPARATOR"
	case LANG_SPEC_LEXER_LANGUAGE_NAME:
		return "LNG NAME"
	case LANG_SPEC_LEXER_VERSION:
		return "VERSION"
	case LANG_SPEC_LEXER_DSL_NAME:
		return "DSL NAME"
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
	LANG_SPEC_ROOT_NODE

	LANG_SPEC_HEADER_NODE

	LANG_SPEC_LANGUAGE_NAME_NODE
	LANG_SPEC_VERSION_NODE
	LANG_SPEC_DSL_NAME_NODE
)

func (k LangSpecParserNodeKind) String() string {
	switch k {
	case LANG_SPEC_ERROR_NODE:
		return "ERROR"
	case LANG_SPEC_ROOT_NODE:
		return "ROOT"
	case LANG_SPEC_HEADER_NODE:
		return "HEADER"
	case LANG_SPEC_LANGUAGE_NAME_NODE:
		return "LNG NAME"
	case LANG_SPEC_VERSION_NODE:
		return "VERSION"
	case LANG_SPEC_DSL_NAME_NODE:
		return "DSL NAME"
	default:
		return "UNKNOWN NODE KIND"
	}
}

// --------------------------------------------------------------- CONFIGURATION

/* LangSpecCompilerConfiguration encapsulates the configuration for the langspec compiler. */
type LangSpecCompilerConfiguration struct {
	scratchAllocationFunction memarch.AllocationFn
	stageReporter             langspec.ASTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]
}

/*
LangSpecCompilerConfigurationCreate creates an instance of the compiler configuration.

Stage reporter is optional.
*/
func LangSpecCompilerConfigurationCreate(
	scratchAllocationFunction memarch.AllocationFn,
	stageReporter langspec.ASTValidationStageSummarizer[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
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
	parser *langspec.LangParser[rune, LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]

	sessionCache *langspec.LangParserSession[rune]
}

/* LangSpecCompilerCreate constructs a compiler instance. */
func LangSpecCompilerCreate(compilerConfig *LangSpecCompilerConfiguration) *LangSpecCompiler {
	spec := buildLangSpecDSLSpec()
	langParserConfig := langspec.LangParserConfigurationCreate(
		spec,
		compilerConfig.scratchAllocationFunction,
		compilerConfig.stageReporter,
	)
	parser := langspec.LangParserCreate(langParserConfig)

	return &LangSpecCompiler{
		parser: parser,
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

	dfaDUMP := compiler.parser.DebugDumpAllLexerDFAs()

	fmt.Println("\n===== DFA DEBUG DUMP =====")
	fmt.Println(dfaDUMP)
	fmt.Println("=========================")

	contentRune, _ := system.FileReadAllRunes(sourceFile)
	fmt.Printf("DEBUG: rune content (escaped):\n%q\n", string(contentRune))
	fmt.Println("DEBUG: rune stream:")
	for i, r := range contentRune {
		fmt.Printf("[%04d] rune=%q  codepoint=U+%04X\n", i, r, r)
	}

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

	trace, rootNode, syntaxErrors, validationEntries, err := langspec.LangParserParseFile(
		compiler.parser,
		session,
	)

	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		renderSyntaxErrorsWithContext(
			contentRune,
			syntaxErrors,
		)
		return fmt.Errorf(
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

	if validationEntries != nil && len(validationEntries.Results) > 0 {
		fmt.Println("\n===== VALIDATION =====")

		for _, stage := range validationEntries.Results {
			fmt.Printf("\n-- Stage: %s (order %d) --\n", stage.StageName, stage.Order)

			if len(stage.Entries) == 0 {
				fmt.Println("  ✔ no issues")
				continue
			}

			for _, entry := range stage.Entries {
				fmt.Printf(
					"  [%v] %s — %s\n",
					entry.Severity,
					entry.Code,
					entry.Message,
				)
			}
		}

		fmt.Println("=======================")
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
		LANG_SPEC_LEXER_ERROR_TOKEN,
		LANG_SPEC_LEXER_EOF_TOKEN,
		LANG_SPEC_LEXER_STATE_DEFAULT,
		lexarch.NewlineDetectorRune(),
		lexarch.RuneFormatterDefault(),
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
		LANG_SPEC_LEXER_LANGUAGE_NAME,
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
		LANG_SPEC_LEXER_DSL_NAME,
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

func buildLangSpecDSLParserSpec() *langspec.ParserSpec[
	rune,
	LangSpecLexerTokenType,
	LangSpecLexerTokenRole,
	LangSpecLexerState,
	LangSpecParserNodeKind,
] {

	parserSpec := langspec.ParserSpecCreate(
		LANG_SPEC_ROOT_NODE,
		LANG_SPEC_ERROR_NODE,

		func(ctx syntaxa.SelectRuleContext[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
		]) syntaxa.ParserRule[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState,
			LangSpecParserNodeKind,
		] {

			// Only valid top-level construct for now = header
			if ctx.Match(LANG_SPEC_LEXER_HEADER_DASHES) {
				fmt.Println("matched dashes")
				return rd.TopLevel(parseHeader())
			}

			return nil
		},

		true, // freeze AST after parse
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_IGNORED_ROLE,
	)

	// ------------------------------------------------------------
	// Validation stages (semantic, not syntactic)
	// ------------------------------------------------------------

	parserSpec.WithValidationStages(
		langspec.ASTValidationStageCreate(
			"header.presence",
			"ensure exactly one language header exists",
			0,

			func(ctx *langspec.ASTValidationStageContext[
				rune,
				LangSpecLexerTokenType,
				LangSpecLexerTokenRole,
				LangSpecParserNodeKind,
			]) {

				headers := ctx.RootNode.FindAllKind(LANG_SPEC_HEADER_NODE)

				if len(headers) == 0 {
					ctx.ReportFatal(
						"missing-header",
						"language specification must contain a header",
						ctx.RootNode,
					)
					return
				}

				if len(headers) > 1 {
					ctx.ReportError(
						"duplicate-header",
						"multiple headers are not allowed",
						headers[1],
					)
				}
			},
		),

		langspec.ASTValidationStageCreate(
			"version.sanity",
			"basic version sanity checks",
			10,

			func(ctx *langspec.ASTValidationStageContext[
				rune,
				LangSpecLexerTokenType,
				LangSpecLexerTokenRole,
				LangSpecParserNodeKind,
			]) {
				// future: semantic version parsing
			},
		),
	)

	return parserSpec
}

func parseHeader() rd.Rule[
	rune,
	LangSpecLexerTokenType,
	LangSpecLexerTokenRole,
	LangSpecLexerState,
	LangSpecParserNodeKind,
] {
	return rd.SequenceAs(
		LANG_SPEC_HEADER_NODE,

		rd.Expect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState,
			LangSpecParserNodeKind,
		](
			LANG_SPEC_LEXER_HEADER_DASHES,
			"expected header start '---'",
		),

		rd.TokenExpect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState](
			LANG_SPEC_LEXER_LANGUAGE_NAME,
			LANG_SPEC_LANGUAGE_NAME_NODE,
			"expected language name",
		),

		rd.TokenExpect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState](
			LANG_SPEC_LEXER_VERSION,
			LANG_SPEC_VERSION_NODE,
			"expected language version",
		),

		rd.Expect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState,
			LangSpecParserNodeKind,
		](
			LANG_SPEC_LEXER_HEADER_SEPARATOR,
			"expected '|' between language and DSL spec",
		),

		rd.TokenExpect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState](
			LANG_SPEC_LEXER_DSL_NAME,
			LANG_SPEC_DSL_NAME_NODE,
			"expected DSL name",
		),

		rd.TokenExpect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState](
			LANG_SPEC_LEXER_VERSION,
			LANG_SPEC_VERSION_NODE,
			"expected DSL version",
		),

		rd.Expect[
			rune,
			LangSpecLexerTokenType,
			LangSpecLexerTokenRole,
			LangSpecLexerState,
			LangSpecParserNodeKind,
		](
			LANG_SPEC_LEXER_HEADER_DASHES,
			"expected header end '---'",
		),
	)
}

func renderSyntaxErrorsWithContext(
	source []rune,
	errors *syntaxa.SyntaxErrors,
) {
	lines := splitLinesRunes(source)

	fmt.Println("\n===== SYNTAX ERRORS =====")

	for _, e := range errors.Errors {
		var typeString = "syntax"
		if e.ProducedByLexer {
			typeString = "lexer"
		}

		fmt.Printf("[%s] %s at %d:%d\n", typeString, e.Message, e.Line, e.Column)

		if e.Line <= 0 || e.Line > len(lines) {
			fmt.Println("  (invalid line reference)")
			continue
		}

		line := lines[e.Line-1]

		fmt.Printf("  %4d | %s\n", e.Line, line)

		if e.Column >= 0 {
			fmt.Printf("       | %s^\n", spaces(e.Column-1))
		}

		fmt.Println()
	}

	fmt.Println("========================")
}

func splitLinesRunes(runes []rune) []string {
	var lines []string
	start := 0

	for i, r := range runes {
		if r == '\n' {
			lines = append(lines, string(runes[start:i]))
			start = i + 1
		}
	}

	if start < len(runes) {
		lines = append(lines, string(runes[start:]))
	}

	return lines
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("%*s", n, "")
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
			"%04d | cur=%d | raw=%s | logical=%s | sel=%v | ok=%v | cons=%v | top=%v | node=%v | rev=%d\n",
			i,
			ev.Cursor,
			formatToken(ev.RawToken),
			formatToken(ev.LogicalToken),
			ev.RuleSelected,
			ev.RuleSucceeded,
			ev.Consumed,
			ev.TopLevel,
			ev.NodeReturned,
			ev.RootRevision,
		)
	}

	fmt.Println("======================")
}
