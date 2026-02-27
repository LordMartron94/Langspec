package editor

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// ============================================================
// FORMATTER (semantic layer)
// ============================================================

/*
EditorIRDebugFormatter supplies string rendering for editor IR debug dumps.

All hooks are optional; missing ones fall back to default formatting (e.g. fmt.Sprintf).
FormatTokenID, FormatContextID, FormatTokenName, and FormatPattern are used for tokens, global trivia,
and context lines. FormatTokenKind is reserved for token-kind display if needed. Color* functions
apply optional styling to the formatted strings. When FormatPattern is set, token and global trivia
lines include the pattern output; otherwise they show "<pattern>".
*/
type EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta any, TTokenKind comparable] struct {
	FormatTokenID   func(TokenID) string
	FormatContextID func(ContextID) string
	FormatTokenKind func(TTokenKind) string
	FormatTokenName func(string) string
	FormatPattern   func(Pattern[TObservation]) string

	ColorTokenID   func(string) string
	ColorContextID func(string) string
	ColorTokenKind func(string) string
}

func (f *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]) tokenID(id TokenID) string {
	if f == nil {
		return fmt.Sprintf("%d", id)
	}
	s := fmt.Sprintf("%d", id)
	if f.FormatTokenID != nil {
		s = f.FormatTokenID(id)
	}
	if f.ColorTokenID != nil {
		s = f.ColorTokenID(s)
	}
	return s
}

func (f *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]) contextID(id ContextID) string {
	if f == nil {
		return fmt.Sprintf("0x%X", id) // Hex is much more readable for uint64 hashes
	}
	s := fmt.Sprintf("0x%X", id)
	if f.FormatContextID != nil {
		s = f.FormatContextID(id)
	}
	if f.ColorContextID != nil {
		s = f.ColorContextID(s)
	}
	return s
}

func (f *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]) tokenKind(k TTokenKind) string {
	if f == nil {
		return fmt.Sprintf("%v", k)
	}
	s := fmt.Sprintf("%v", k)
	if f.FormatTokenKind != nil {
		s = f.FormatTokenKind(k)
	}
	if f.ColorTokenKind != nil {
		s = f.ColorTokenKind(s)
	}
	return s
}

func (f *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]) tokenName(name string) string {
	if f != nil && f.FormatTokenName != nil {
		return f.FormatTokenName(name)
	}
	return name
}

func (f *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]) pattern(p Pattern[TObservation]) string {
	if f != nil && f.FormatPattern != nil {
		return f.FormatPattern(p)
	}
	return "<pattern>"
}

// ============================================================
// RENDERER (layout + IO)
// ============================================================

/*
EditorIRDebugger renders an EditorIR to a human-readable dump.

Output includes start context, token definitions, global trivia, and contexts
with boundaries and valid paths. Use NewEditorIRDebugger to construct.
*/
type EditorIRDebugger[TObservation, TTokenMeta, TContextMeta any, TTokenKind comparable] struct {
	Formatter *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]
}

/*
NewEditorIRDebugger creates an EditorIRDebugger with the given formatter.

Formatter may be nil; a nil formatter uses default formatting for all fields (e.g. numeric IDs, "<pattern>").
*/
func NewEditorIRDebugger[TObservation, TTokenMeta, TContextMeta any, TTokenKind comparable](
	formatter *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind],
) *EditorIRDebugger[TObservation, TTokenMeta, TContextMeta, TTokenKind] {
	if formatter == nil {
		formatter = &EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]{}
	}
	return &EditorIRDebugger[TObservation, TTokenMeta, TContextMeta, TTokenKind]{
		Formatter: formatter,
	}
}

/*
DumpTo writes the full debug dump of the editor IR to w.

Sections: overview (start context), tokens (with pattern when formatter provides FormatPattern),
global trivia (with pattern when formatter provides FormatPattern), and contexts (with boundaries
and valid paths). Returns any write error. If ir is nil, writes "<nil>\n".
*/
func (d *EditorIRDebugger[TObservation, TTokenMeta, TContextMeta, TTokenKind]) DumpTo(
	w io.Writer,
	ir *EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind],
) error {
	if ir == nil {
		_, err := io.WriteString(w, "<nil>\n")
		return err
	}

	f := d.Formatter
	if f == nil {
		f = &EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind]{}
	}

	// Pre-compute token name and definition maps for readable boundaries, paths, and trivia
	tokenNames := make(map[TokenID]string, len(ir.tokens))
	tokenByID := make(map[TokenID]*TokenDefinition[TObservation, TTokenMeta, TTokenKind], len(ir.tokens))
	for i := range ir.tokens {
		td := &ir.tokens[i]
		tokenNames[td.ID] = f.tokenName(td.Name)
		tokenByID[td.ID] = td
	}

	// Helper to format a token ID as "Name (ID)" for readable path/boundary dumps
	formatTokenRef := func(tid TokenID) string {
		name, ok := tokenNames[tid]
		if !ok {
			name = "UNKNOWN"
		}
		return fmt.Sprintf("%s (%s)", name, f.tokenID(tid))
	}

	if _, err := io.WriteString(w, "=== Editor IR ===\n"); err != nil {
		return err
	}
	startCtx := ir.contexts[ir.start]
	startLine := f.contextID(ir.start)
	if startCtx != nil && startCtx.Name != "" {
		startLine = startCtx.Name + " " + startLine
	}
	if _, err := fmt.Fprintf(w, "  start context: %s\n\n", startLine); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "=== Tokens ===\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  count: %d\n", len(ir.tokens)); err != nil {
		return err
	}
	for _, td := range ir.tokens {
		line := fmt.Sprintf("  - [%s] %q (priority: %d) %s",
			f.tokenID(td.ID), f.tokenName(td.Name), td.Priority, f.pattern(td.Pattern))
		if _, err := io.WriteString(w, line+"\n"); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "=== Global trivia ===\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  count: %d\n", len(ir.globalTrivia)); err != nil {
		return err
	}
	for _, tid := range ir.globalTrivia {
		line := formatTokenRef(tid)
		if td := tokenByID[tid]; td != nil {
			line += " " + f.pattern(td.Pattern)
		}
		if _, err := fmt.Fprintf(w, "  - %s\n", line); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	if _, err := io.WriteString(w, "=== Contexts ===\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  count: %d\n", len(ir.contexts)); err != nil {
		return err
	}

	ctxIDs := sortedContextIDs(ir.contexts)
	for _, ctxID := range ctxIDs {
		ctx := ir.contexts[ctxID]
		if ctx == nil {
			if _, err := fmt.Fprintf(w, "  - %s: <nil>\n", f.contextID(ctxID)); err != nil {
				return err
			}
			continue
		}

		finalMark := ""
		if ctx.IsFinal {
			finalMark = " (final)"
		}

		ctxLine := f.contextID(ctx.ID)
		if ctx.Name != "" {
			ctxLine = ctx.Name + " " + ctxLine
		}
		if _, err := fmt.Fprintf(w, "  - Context %s%s\n", ctxLine, finalMark); err != nil {
			return err
		}

		if len(ctx.Boundaries) > 0 {
			if _, err := io.WriteString(w, "    boundaries:\n"); err != nil {
				return err
			}
			for _, b := range ctx.Boundaries {
				action := "-> EXIT"
				if !b.IsExit {
					action = fmt.Sprintf("-> ENTER %s", f.contextID(b.TargetContext))
				}
				if _, err := fmt.Fprintf(w, "      * trigger: %s %s\n", formatTokenRef(b.OnTrigger), action); err != nil {
					return err
				}
			}
		}

		if len(ctx.ValidPaths) > 0 {
			if _, err := io.WriteString(w, "    valid paths:\n"); err != nil {
				return err
			}
			for _, seq := range ctx.ValidPaths {
				pathStrs := make([]string, 0, len(seq.ExpectedPath))
				for _, pe := range seq.ExpectedPath {
					if pe.Type == ElementToken {
						pathStrs = append(pathStrs, formatTokenRef(pe.Token))
					} else {
						pathStrs = append(pathStrs, fmt.Sprintf("REF %s", f.contextID(pe.TargetContext)))
					}
				}
				if _, err := io.WriteString(w, "      * ["+strings.Join(pathStrs, " -> ")+"]\n"); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

/*
DumpString returns the full debug dump of the editor IR as a string.
Equivalent to DumpTo with a strings.Builder.
*/
func (d *EditorIRDebugger[TObservation, TTokenMeta, TContextMeta, TTokenKind]) DumpString(
	ir *EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind],
) string {
	var b strings.Builder
	_ = d.DumpTo(&b, ir)
	return b.String()
}

func sortedContextIDs[TMeta any](contexts map[ContextID]*EditorContext[TMeta]) []ContextID {
	ids := make([]ContextID, 0, len(contexts))
	for id := range contexts {
		ids = append(ids, id)
	}
	// Sort uint64 keys numerically to ensure deterministic dumps
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

// ============================================================
// EditorIR API convenience
// ============================================================

/*
DebugDump produces a human-readable dump of the editor IR using the given formatter.

Convenience wrapper around NewEditorIRDebugger and DumpString. Formatter may be nil for default
formatting. Tokens and global trivia show pattern via the formatter when FormatPattern is set.
*/
func (ir *EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind]) DebugDump(
	formatter *EditorIRDebugFormatter[TObservation, TTokenMeta, TContextMeta, TTokenKind],
) string {
	dbg := NewEditorIRDebugger(formatter)
	return dbg.DumpString(ir)
}
