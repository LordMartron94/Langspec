package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"foundation/hash"
	"foundation/text"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"langspec/editor"
	"langspec/editor/sublime"
	"langspec/toolchain"
	"lexarch"
)

/*
TestMultistateLexFixtureSublimePath compiles a small .lspec with INITIAL and INNER lexer states
linked by [push(INNER)] / [pop(1)], then checks that the generated .sublime-syntax actually
contains the highlighting grammar implied by that multi-state lexer together with the parse
machine: the regex for each token is taken from the lexer rule in the correct state, and
stack mutations show up as Sublime push/pop alongside the matching rules.
*/
func TestMultistateLexFixtureSublimePath(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	lspecPath := filepath.Join("testdata", "multistate_lex_minimal.lspec")
	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, lspecPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile: %v", err)
	}
	if res == nil {
		t.Fatal("nil compile result")
	}
	sym := res.CompiledSymbols
	if sym == nil {
		t.Fatal("nil CompiledSymbols")
	}

	editorRuleset := toolchain.LexerSpecToEditorLexingRuleSet(res.CompiledLexerSpec)
	if editorRuleset == nil {
		t.Fatal("nil editor ruleset from compiled lexer spec")
	}

	hasher := hash.XXH3HasherCreateWithSeed(6789)
	ctxProducer := func(*editor.EditorCtx[rune, uint32, uint32, string, dsl.LangSpecParserNodeKind]) toolchain.SublimeContext {
		return toolchain.SublimeContext{Scope: "source.multistate.test"}
	}
	overrideProducer := func(*editor.EditorCtx[rune, uint32, uint32, string, dsl.LangSpecParserNodeKind]) []*editor.EditorOverride[rune, uint32, uint32, string, dsl.LangSpecParserNodeKind, toolchain.SublimeContext] {
		return nil
	}
	irConfig := editor.EditorIRConfigurationCreate(
		hasher,
		func(tok lexarch.TokenKind) uint64 { return uint64(tok) },
		func(nk dsl.LangSpecParserNodeKind) uint64 { return uint64(nk) },
		ctxProducer,
		overrideProducer,
		func(a, b toolchain.SublimeContext) bool { return a.Scope == b.Scope && a.MetaScope == b.MetaScope },
	)

	editorIR, err := editor.EditorIRCreate(editorRuleset, &res.CompiledGrammarPackage, irConfig)
	if err != nil {
		t.Fatalf("EditorIRCreate: %v", err)
	}

	outPath := filepath.Join(t.TempDir(), "multistate.sublime-syntax")
	ext := sublime.ExtractionConfig[toolchain.SublimeContext]{
		ExtractScope: func(ctx toolchain.SublimeContext) string {
			return ctx.Scope
		},
		ExtractMetaScope: func(ctx toolchain.SublimeContext) string {
			return ctx.MetaScope
		},
	}
	if err := sublime.GenerateSyntaxFile(editorIR, []string{".mt"}, "source.multistate.test", outPath, ext, nil); err != nil {
		t.Fatalf("GenerateSyntaxFile: %v", err)
	}

	body, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated syntax: %v", err)
	}
	out := string(body)

	openRE := multistateLexRuleRegex(t, editorRuleset, sym, "TokOpen", "INITIAL")
	innerRE := multistateLexRuleRegex(t, editorRuleset, sym, "TokInner", "INNER")
	closeRE := multistateLexRuleRegex(t, editorRuleset, sym, "TokClose", "INNER")

	// Highlighting grammar must embed the same regex the lexer uses per state (parser-sparse paths).
	if !strings.Contains(out, openRE) {
		t.Fatalf("generated syntax missing INITIAL TokOpen pattern %q (open delimiter)", openRE)
	}
	if !strings.Contains(out, innerRE) {
		t.Fatalf("generated syntax missing INNER TokInner pattern %q; multi-state lexer is not driving this match", innerRE)
	}
	if !strings.Contains(out, closeRE) {
		t.Fatalf("generated syntax missing INNER TokClose pattern %q (close delimiter)", closeRE)
	}

	san := text.NewIdentifierSanitizer()
	innerSublimeCtx := "lex__" + san.Sanitize("INNER")
	if !strings.Contains(out, innerSublimeCtx) {
		t.Fatalf("generated syntax missing lex stack target %q for [push(INNER)] on TokOpen", innerSublimeCtx)
	}
	if !strings.Contains(out, "pop:") {
		t.Fatalf("generated syntax missing pop: for [pop(1)] on TokClose")
	}
}

func multistateLexRuleRegex(
	t *testing.T,
	rs *editor.LexingRuleSet[rune, uint32, uint32],
	sym *semantics.CompiledSymbolTable,
	tokenName string,
	lexerState string,
) string {
	t.Helper()
	tok := sym.TokenID(tokenName)
	if tok == 0 {
		t.Fatalf("unknown token name %q in compiled symbols", tokenName)
	}
	for _, r := range editor.LexingRuleSetGetRules(rs) {
		if r.Token != tok {
			continue
		}
		if r.LexerState != lexerState {
			continue
		}
		re, err := r.Pattern.ToRegEx()
		if err != nil {
			t.Fatalf("ToRegEx for %s in state %s: %v", tokenName, lexerState, err)
		}
		return re
	}
	t.Fatalf("no lex rule for token %q in lexer state %q", tokenName, lexerState)
	return ""
}
