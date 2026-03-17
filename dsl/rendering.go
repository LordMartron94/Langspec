package dsl

import (
	"fmt"
	"foundation/formatting"
	"io"
	"langspec/validation"
	"lexarch"
	"strings"
	"syntaxa"
	"text/tabwriter"
)

// -----------------------------------------------------------------------------
// SYNTAX ERRORS
// -----------------------------------------------------------------------------

func RenderSyntaxErrorsWithContext(
	w io.Writer,
	source []rune,
	errs *syntaxa.SyntaxErrors[rune],
	advanceFn lexarch.ColumnAdvanceFn[rune],
) {
	if w == nil || errs == nil || len(errs.Errors) == 0 {
		return
	}

	lines := splitLinesRunes(source)

	fmt.Fprintln(w, "\n===== SYNTAX ERRORS =====")

	for _, e := range errs.Errors {
		renderSingleSyntaxError(w, e, lines, advanceFn)
	}

	fmt.Fprintln(w, "========================")
}

func renderSingleSyntaxError(w io.Writer, e syntaxa.SyntaxError[rune], lines [][]rune, advanceFn lexarch.ColumnAdvanceFn[rune]) {
	printSyntaxErrorHeader(w, e)
	renderDiagnosticContext(w, lines, e.StartLine, e.StartColumn, e.EndLine, e.EndColumn, advanceFn)
}

func printSyntaxErrorHeader(w io.Writer, e syntaxa.SyntaxError[rune]) {
	typeStr := "syntax"
	if e.ProducedByLexer {
		typeStr = "lexer"
	}
	fmt.Fprintf(w, "[%s] (rule=%s) %s at %d:%d\n", typeStr, e.Rule, e.Message, e.StartLine, e.StartColumn)
}

// -----------------------------------------------------------------------------
// VALIDATION ENTRIES
// -----------------------------------------------------------------------------

func renderValidationEntries(
	w io.Writer,
	source []rune,
	entries *validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
	advanceFn lexarch.ColumnAdvanceFn[rune],
) {
	if w == nil || entries == nil || len(entries.Results) == 0 {
		return
	}

	lines := splitLinesRunes(source)

	fmt.Fprintln(w, "\n===== VALIDATION =====")

	for _, stage := range entries.Results {
		renderValidationStage(w, stage, lines, advanceFn)
	}

	fmt.Fprintln(w, "=======================")
}

func renderValidationStage(
	w io.Writer,
	stage validation.StageValidationResult[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
	lines [][]rune,
	advanceFn lexarch.ColumnAdvanceFn[rune],
) {
	fmt.Fprintf(w, "\n-- Stage: %s (order %d) --\n", stage.StageName, stage.Order)

	if len(stage.Entries) == 0 {
		fmt.Fprintln(w, "  ✔ no issues")
		return
	}

	for _, entry := range stage.Entries {
		renderValidationEntry(w, entry, lines, advanceFn)
	}
}

func renderValidationEntry(
	w io.Writer,
	entry validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
	lines [][]rune,
	advanceFn lexarch.ColumnAdvanceFn[rune],
) {
	fmt.Fprintf(w, "  [%v] %s — %s\n", entry.Severity, entry.Code, entry.Message)

	if entry.Node == nil {
		renderFallbackValidationSpan(w, entry)
		return
	}

	startLine, startCol, endLine, endCol := entry.Node.LineSpan()
	fmt.Fprintf(w, "      Location: line %d:%d to %d:%d (Node Kind: %v, ID: %d)\n",
		startLine, startCol, endLine, endCol, entry.Node.Kind(), entry.Node.ID())

	renderDiagnosticContext(w, lines, startLine, startCol, endLine, endCol, advanceFn)
}

func renderFallbackValidationSpan(
	w io.Writer,
	entry validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
) {
	if entry.Start != 0 || entry.End != 0 {
		fmt.Fprintf(w, "      Absolute Span: %d - %d\n", entry.Start, entry.End)
	}
}

// -----------------------------------------------------------------------------
// SHARED DIAGNOSTIC HIGHLIGHTING
// -----------------------------------------------------------------------------

func renderDiagnosticContext(w io.Writer, lines [][]rune, startL, startC, endL, endC int, advanceFn lexarch.ColumnAdvanceFn[rune]) {
	if !isValidSpanRange(startL, endL, len(lines)) {
		renderInvalidSpanBlock(w, lines, startL, startC, endL, endC, advanceFn)
		return
	}

	if startL == endL {
		renderSingleLineHighlight(w, lines[startL-1], startL, startC, endC, advanceFn)
		return
	}

	renderMultiLineHighlight(w, lines, startL, startC, endL, endC, advanceFn)
}

func isValidSpanRange(startLine, endLine, totalLines int) bool {
	if startLine <= 0 || endLine <= 0 || startLine > endLine {
		return false
	}
	return isValidLine(startLine, totalLines) && isValidLine(endLine, totalLines)
}

func renderSingleLineHighlight(
	w io.Writer,
	line []rune,
	lineNum, startCol, endCol int,
	advanceFn lexarch.ColumnAdvanceFn[rune],
) {
	var sourceBuilder strings.Builder
	var markerBuilder strings.Builder

	currentCol := 1

	for _, r := range line {
		nextCol := advanceFn(r, currentCol)
		width := nextCol - currentCol

		sourceBuilder.WriteString(sanitizeRune(r, width))
		markerBuilder.WriteString(buildMarkerFragment(currentCol, startCol, endCol, width))

		currentCol = nextCol
	}

	// Handle tokens (like EOF) that extend past the physical line break
	if currentCol <= startCol {
		markerBuilder.WriteString(strings.Repeat(" ", startCol-currentCol))
		markerBuilder.WriteString("^")
	} else if currentCol < endCol {
		markerBuilder.WriteString(strings.Repeat("~", endCol-currentCol))
	}

	fmt.Fprintf(w, " %4d | %s\n", lineNum, sourceBuilder.String())
	fmt.Fprint(w, "      | ")
	fmt.Fprintln(w, markerBuilder.String())
}

func sanitizeRune(r rune, width int) string {
	if r == '\t' {
		return strings.Repeat(" ", width)
	}
	return string(r)
}

func buildMarkerFragment(currentCol, startCol, endCol, width int) string {
	if currentCol < startCol {
		return strings.Repeat(" ", width)
	}

	// Guarantee the caret renders the exact moment we hit the target
	if currentCol == startCol {
		if width > 1 && endCol > startCol {
			return "^" + strings.Repeat("~", width-1)
		}
		return "^"
	}

	if currentCol < endCol {
		return strings.Repeat("~", width)
	}

	return ""
}

func renderMultiLineHighlight(w io.Writer, lines [][]rune, startL, startC, endL, endC int, advanceFn lexarch.ColumnAdvanceFn[rune]) {
	firstLine := lines[startL-1]
	renderSingleLineHighlight(w, firstLine, startL, startC, len(firstLine)+1, advanceFn)

	if endL > startL+1 {
		fmt.Fprintln(w, "      | ...")
	}

	lastLine := lines[endL-1]
	renderSingleLineHighlight(w, lastLine, endL, 1, endC, advanceFn)
}

func renderInvalidSpanBlock(w io.Writer, lines [][]rune, startL, startC, endL, endC int, advanceFn lexarch.ColumnAdvanceFn[rune]) {
	fmt.Fprintln(w, " ── INVALID OR OUT-OF-BOUNDS SPAN DETECTED ────────────────")
	fmt.Fprintf(w, "  Requested: %d:%d to %d:%d | Total lines: %d\n", startL, startC, endL, endC, len(lines))

	lineIdx := determineFallbackLineIndex(startL, len(lines))
	if lineIdx < 0 {
		fmt.Fprintln(w, "(no source available)")
		return
	}

	renderSingleLineHighlight(w, lines[lineIdx], lineIdx+1, startC, endC, advanceFn)
	fmt.Fprintln(w, " ──────────────────────────────────────────────────────────")
}

func determineFallbackLineIndex(startLine, totalLines int) int {
	if startLine > 0 && startLine <= totalLines {
		return startLine - 1
	}
	if totalLines > 0 {
		return totalLines - 1
	}
	return -1
}

func enforceMinimumColumn(col int) int {
	if col < 1 {
		return 1
	}
	return col
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

// -----------------------------------------------------------------------------
// DEBUG DUMPS & TRACES
// -----------------------------------------------------------------------------

func RenderParseTrace[TToken any](
	w io.Writer,
	trace *syntaxa.ParseTrace[TToken],
	formatToken func(TToken) string,
) {
	if w == nil {
		return
	}
	if trace == nil || len(trace.Events) == 0 {
		fmt.Fprintln(w, "\n===== PARSE TRACE =====")
		fmt.Fprintln(w, "  (no trace data)")
		fmt.Fprintln(w, "======================")
		return
	}

	fmt.Fprintln(w, "\n===== PARSE TRACE =====")

	// Initialize tabwriter for aligned columns: minwidth, tabwidth, padding, padchar, flags
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	fmt.Fprintln(tw, "IDX\tCUR\tRULE\tOK\tCONS\tNODE\tRAW / LOGICAL\tRECOVERY\tSYNC SET")
	fmt.Fprintln(tw, "---\t---\t----\t--\t----\t----\t-------------\t--------\t--------")

	for i, ev := range trace.Events {
		ok := "Y"
		if !ev.RuleSucceeded {
			ok = "N"
		}
		cons := "Y"
		if !ev.Consumed {
			cons = "N"
		}
		node := "Y"
		if !ev.NodeReturned {
			node = "N"
		}

		tokens := fmt.Sprintf("%s / %s", formatToken(ev.RawToken), formatToken(ev.LogicalToken))
		recStatus := formatRecoveryStatus(ev.RecoveryAttempted, ev.Recovered, ev.LandedOnOurs)
		syncSet := formatting.FormatSlice(ev.RecoveryTokenSet, formatting.FormatSliceOptions[TToken]{
			FormatItem: func(index int, value TToken) string {
				return formatToken(value)
			},
			Prefix:       "[",
			Suffix:       "]",
			IncludeIndex: true,
		})

		fmt.Fprintf(tw,
			"%04d\t%d\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			i,
			ev.Cursor,
			ev.RuleName,
			ok,
			cons,
			node,
			tokens,
			recStatus,
			syncSet,
		)
	}

	tw.Flush()
	fmt.Fprintln(w, "======================")
}

func formatRecoveryStatus(attempted, recovered, landed bool) string {
	if !attempted {
		return "-"
	}
	if recovered {
		if landed {
			return "RECOVERED (SYNCED)"
		}
		return "RECOVERED (DELEGATED)"
	}
	return "FAILED"
}

func RenderLSTDump(w io.Writer, dump string) {
	if w == nil || dump == "" {
		return
	}
	fmt.Fprintln(w, "\n===== LST DEBUG DUMP =====")
	fmt.Fprintln(w, dump)
	fmt.Fprintln(w, "=========================")
}

func RenderGrammarDumps(w io.Writer, grammarDump, grammarPackageDump, cfgDump string) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, "\n===== GRAMMAR DEBUG DUMP =====")
	fmt.Fprintln(w, grammarDump)
	fmt.Fprintln(w, "=========================")
	fmt.Fprintln(w, "\n===== GRAMMAR PACKAGE DEBUG DUMP =====")
	fmt.Fprintln(w, grammarPackageDump)
	fmt.Fprintln(w, "=========================")
	if cfgDump != "" {
		fmt.Fprintln(w, "\n===== CFG (Contexta) DEBUG DUMP =====")
		fmt.Fprintln(w, cfgDump)
		fmt.Fprintln(w, "=========================")
	}
}

func renderLexemes(
	w io.Writer,
	lexemes []lexarch.Lexeme[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	formatToken func(LangSpecLexerTokenType) string,
	formatRole func(LangSpecLexerTokenRole) string,
) {
	if w == nil || len(lexemes) == 0 {
		return
	}
	for i, lexeme := range lexemes {
		debug := lexeme.DebugString(formatToken, formatRole)
		fmt.Fprintf(w, "%05d) %s\n", i, debug)
	}
}
