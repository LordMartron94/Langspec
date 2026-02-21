package dsl

import (
	"fmt"
	"strings"
	"syntaxa"
)

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
