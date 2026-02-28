package dsl

import (
	"fmt"
	"io"
	"langspec/validation"
	"lexarch"
	"strings"
	"syntaxa"
)

func renderSyntaxErrorsWithContext(
	w io.Writer,
	source []rune,
	errs *syntaxa.SyntaxErrors[rune],
) {
	if w == nil || len(errs.Errors) == 0 {
		return
	}

	lines := splitLinesRunes(source)

	fmt.Fprintln(w, "\n===== SYNTAX ERRORS =====")

	for _, e := range errs.Errors {
		printErrorHeaderTo(w, e)

		if e.StartLine != e.EndLine || e.StartLine <= 0 || !isValidLine(e.StartLine, len(lines)) {
			fmt.Fprintln(w, " ── INVALID ERROR SPAN DETECTED ─────────────────────────────")
			fmt.Fprintf(w, "  Message: %s\n", e.Message)
			fmt.Fprintf(w, "  StartLine: %d  StartColumn: %d\n", e.StartLine, e.StartColumn)
			fmt.Fprintf(w, "  EndLine:   %d  EndColumn:   %d\n", e.EndLine, e.EndColumn)
			fmt.Fprintf(w, "  Total lines: %d\n", len(lines))

			var lineIdx int
			switch {
			case e.StartLine > 0 && e.StartLine <= len(lines):
				lineIdx = e.StartLine - 1
			case len(lines) > 0:
				lineIdx = len(lines) - 1
			default:
				fmt.Fprintln(w, "(no source available)")
				fmt.Fprintln(w)
				continue
			}

			line := lines[lineIdx]
			fmt.Fprintf(w, " %4d | %s\n", lineIdx+1, string(line))
			fmt.Fprint(w, "      | ")
			fmt.Fprintln(w, renderSpan(line, e.StartColumn, e.EndColumn, 4))
			fmt.Fprintln(w, " ──────────────────────────────────────────────────────────")
			continue
		}

		line := lines[e.StartLine-1]
		fmt.Fprintf(w, " %4d | %s\n", e.StartLine, string(line))
		fmt.Fprint(w, "      | ")
		fmt.Fprintln(w, renderSpan(line, e.StartColumn, e.EndColumn, 4))
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "========================")
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

func printErrorHeaderTo(w io.Writer, e syntaxa.SyntaxError[rune]) {
	if w == nil {
		return
	}
	typeStr := "syntax"
	if e.ProducedByLexer {
		typeStr = "lexer"
	}
	fmt.Fprintf(w, "[%s] (rule=%s) %s at %d:%d\n", typeStr, e.Rule, e.Message, e.StartLine, e.StartColumn)
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

	fmt.Fprintln(w, "======================")
}

func renderValidationEntries(
	w io.Writer,
	entries *validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind],
) {
	if w == nil || entries == nil || len(entries.Results) == 0 {
		return
	}

	fmt.Fprintln(w, "\n===== VALIDATION =====")

	for _, stage := range entries.Results {
		fmt.Fprintf(w, "\n-- Stage: %s (order %d) --\n", stage.StageName, stage.Order)

		if len(stage.Entries) == 0 {
			fmt.Fprintln(w, "  ✔ no issues")
			continue
		}

		for _, entry := range stage.Entries {
			fmt.Fprintf(w,
				"  [%v] %s — %s\n",
				entry.Severity,
				entry.Code,
				entry.Message,
			)
		}
	}

	fmt.Fprintln(w, "=======================")
}

func renderASTDump(w io.Writer, dump string) {
	if w == nil || dump == "" {
		return
	}
	fmt.Fprintln(w, "\n===== AST DEBUG DUMP =====")
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
