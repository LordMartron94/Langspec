package dsl

import (
	"fmt"
	"io"
	"langspec/validation"
	"lexarch"
	"strings"
	"syntaxa"
)

// -----------------------------------------------------------------------------
// SYNTAX ERRORS
// -----------------------------------------------------------------------------

func renderSyntaxErrorsWithContext(
	w io.Writer,
	source []rune,
	errs *syntaxa.SyntaxErrors[rune],
) {
	if w == nil || errs == nil || len(errs.Errors) == 0 {
		return
	}

	lines := splitLinesRunes(source)

	fmt.Fprintln(w, "\n===== SYNTAX ERRORS =====")

	for _, e := range errs.Errors {
		renderSingleSyntaxError(w, e, lines)
	}

	fmt.Fprintln(w, "========================")
}

func renderSingleSyntaxError(w io.Writer, e syntaxa.SyntaxError[rune], lines [][]rune) {
	printSyntaxErrorHeader(w, e)
	renderDiagnosticContext(w, lines, e.StartLine, e.StartColumn, e.EndLine, e.EndColumn)
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
) {
	if w == nil || entries == nil || len(entries.Results) == 0 {
		return
	}

	lines := splitLinesRunes(source)

	fmt.Fprintln(w, "\n===== VALIDATION =====")

	for _, stage := range entries.Results {
		renderValidationStage(w, stage, lines)
	}

	fmt.Fprintln(w, "=======================")
}

func renderValidationStage(
	w io.Writer,
	stage validation.StageValidationResult[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
	lines [][]rune,
) {
	fmt.Fprintf(w, "\n-- Stage: %s (order %d) --\n", stage.StageName, stage.Order)

	if len(stage.Entries) == 0 {
		fmt.Fprintln(w, "  ✔ no issues")
		return
	}

	for _, entry := range stage.Entries {
		renderValidationEntry(w, entry, lines)
	}
}

func renderValidationEntry(
	w io.Writer,
	entry validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
	lines [][]rune,
) {
	fmt.Fprintf(w, "  [%v] %s — %s\n", entry.Severity, entry.Code, entry.Message)

	if entry.Node == nil {
		renderFallbackValidationSpan(w, entry)
		return
	}

	startLine, startCol, endLine, endCol := entry.Node.LineSpan()
	fmt.Fprintf(w, "      Location: line %d:%d to %d:%d (Node Kind: %v, ID: %d)\n",
		startLine, startCol, endLine, endCol, entry.Node.Kind(), entry.Node.ID())

	renderDiagnosticContext(w, lines, startLine, startCol, endLine, endCol)
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

func renderDiagnosticContext(w io.Writer, lines [][]rune, startL, startC, endL, endC int) {
	if !isValidSpanRange(startL, endL, len(lines)) {
		renderInvalidSpanBlock(w, lines, startL, startC, endL, endC)
		return
	}

	if startL == endL {
		renderSingleLineHighlight(w, lines[startL-1], startL, startC, endC)
		return
	}

	renderMultiLineHighlight(w, lines, startL, startC, endL, endC)
}

func isValidSpanRange(startLine, endLine, totalLines int) bool {
	if startLine <= 0 || endLine <= 0 || startLine > endLine {
		return false
	}
	return isValidLine(startLine, totalLines) && isValidLine(endLine, totalLines)
}

func renderSingleLineHighlight(w io.Writer, line []rune, lineNum, startCol, endCol int) {
	fmt.Fprintf(w, " %4d | %s\n", lineNum, string(line))
	fmt.Fprint(w, "      | ")
	fmt.Fprintln(w, renderSpanMarker(line, startCol, endCol, 4))
	fmt.Fprintln(w)
}

func renderMultiLineHighlight(w io.Writer, lines [][]rune, startL, startC, endL, endC int) {
	firstLine := lines[startL-1]
	renderSingleLineHighlight(w, firstLine, startL, startC, len(firstLine)+1)

	if endL > startL+1 {
		fmt.Fprintln(w, "      | ...")
	}

	lastLine := lines[endL-1]
	renderSingleLineHighlight(w, lastLine, endL, 1, endC)
}

func renderInvalidSpanBlock(w io.Writer, lines [][]rune, startL, startC, endL, endC int) {
	fmt.Fprintln(w, " ── INVALID OR OUT-OF-BOUNDS SPAN DETECTED ────────────────")
	fmt.Fprintf(w, "  Requested: %d:%d to %d:%d | Total lines: %d\n", startL, startC, endL, endC, len(lines))

	lineIdx := determineFallbackLineIndex(startL, len(lines))
	if lineIdx < 0 {
		fmt.Fprintln(w, "(no source available)")
		return
	}

	renderSingleLineHighlight(w, lines[lineIdx], lineIdx+1, startC, endC)
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

func renderSpanMarker(line []rune, startCol, endCol int, tabWidth int) string {
	startCol = enforceMinimumColumn(startCol)
	endCol = enforceMinimumColumn(endCol)

	if endCol < startCol {
		endCol = startCol
	}

	visualStart := calculateVisualOffset(line, startCol, tabWidth)
	visualEnd := calculateVisualOffset(line, endCol, tabWidth)

	var sb strings.Builder
	sb.WriteString(strings.Repeat(" ", visualStart))

	spanWidth := visualEnd - visualStart
	if spanWidth <= 1 {
		sb.WriteString("^")
		return sb.String()
	}

	sb.WriteString("^")
	sb.WriteString(strings.Repeat("~", spanWidth-1))
	return sb.String()
}

func enforceMinimumColumn(col int) int {
	if col < 1 {
		return 1
	}
	return col
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

func renderParseTrace[TToken any](
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

	for i, ev := range trace.Events {
		fmt.Fprintf(w,
			"%04d | cur=%d | raw=%s | logical=%s | ok=%v | cons=%v | node=%v | rule=%s\n",
			i,
			ev.Cursor,
			formatToken(ev.RawToken),
			formatToken(ev.LogicalToken),
			ev.RuleSucceeded,
			ev.Consumed,
			ev.NodeReturned,
			string(ev.RuleName),
		)
	}

	fmt.Fprintln(w, "======================")
}

func renderLSTDump(w io.Writer, dump string) {
	if w == nil || dump == "" {
		return
	}
	fmt.Fprintln(w, "\n===== LST DEBUG DUMP =====")
	fmt.Fprintln(w, dump)
	fmt.Fprintln(w, "=========================")
}

func renderGrammarDumps(w io.Writer, grammarDump, grammarPackageDump string) {
	if w == nil {
		return
	}
	fmt.Fprintln(w, "\n===== GRAMMAR DEBUG DUMP =====")
	fmt.Fprintln(w, grammarDump)
	fmt.Fprintln(w, "=========================")
	fmt.Fprintln(w, "\n===== GRAMMAR PACKAGE DEBUG DUMP =====")
	fmt.Fprintln(w, grammarPackageDump)
	fmt.Fprintln(w, "=========================")
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
