package tests

import (
	"langspec"
	"langspec/bootstrap"
	"langspec/dsl"
	dsleditor "langspec/dsl/editor"
	dslspec "langspec/dsl/spec"
	"langspec/editor"
	"langspec/toolchain"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Same canonical example as compiler_tests / examples/lspec.lspec PRAGMA (go_bindings → libs/langspec/examples/generated_go_bindings.go).
const testClientDSLFile = "libs/langspec/examples/lspec.lspec"

func TestClientDSLToolchain(t *testing.T) {
	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	compileResult, err := dsl.LangSpecCompilerCompile(compiler, testClientDSLFile)
	if err != nil {
		t.Fatalf("Client compilation failed with error: %s", err.Error())
	}

	sublimePragma, ok := compileResult.CompiledToolPragmas[toolchain.SublimeToolName]
	if !ok || sublimePragma.Settings[toolchain.SublimeEnableKey] != "true" {
		t.Skip("Sublime toolchain not enabled in spec pragma")
	}

	stringManifest := adaptManifest(dsleditor.LangSpecEditorManifest, dsl.LangSpecCompilerScopeMap(compiler))

	err = bootstrap.RunToolchainsFromCompileResult(
		compileResult,
		bootstrap.WithSublimeToolchain(
			stringManifest,
			func(
				_ *editor.LexingRuleSet[rune, uint32, uint32],
				_ func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) toolchain.SublimeContext,
			) func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, toolchain.SublimeContext] {
				return adaptGoOverrideProducer(compileResult, compiler)
			},
			[]string{".lspec"},
			".lspec",
		),
	)
	if err != nil {
		t.Fatalf("Toolchains failed: %s", err.Error())
	}
}

func TestToolchainGoBindingsNamespacesImportedSymbols(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modAPath := filepath.Join(tmpDir, "module_a_bindings.lspec")
	modBPath := filepath.Join(tmpDir, "module_b_bindings.lspec")
	outPath := filepath.Join(tmpDir, "generated_bindings.go")
	mainPath := filepath.Join(tmpDir, "main_bindings.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
LEX {
	state INITIAL {
		TokShared -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	export rule SharedRule -> SharedNode { virtual TokShared };
}`
	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---
PRAGMA {
	tool.go_bindings {
		enable = true;
		output-path = "` + outPath + `";
		package-name = "tmpbindings";
	}
}
IMPORT {
	"` + modAPath + `" as A;
	"` + modBPath + `" as B;
}
LEX {
	state INITIAL {
		TokMain -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { word };
	rule PROGRAM -> ProgramNode {
		using A.SharedRule
		using B.SharedRule
	};
}`

	if err := os.WriteFile(modAPath, []byte(module), 0644); err != nil {
		t.Fatalf("write module A spec: %v", err)
	}
	if err := os.WriteFile(modBPath, []byte(module), 0644); err != nil {
		t.Fatalf("write module B spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
	if err != nil {
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}

	if err := toolchain.RunGoBindingsToolchain(res); err != nil {
		t.Fatalf("RunGoBindingsToolchain failed: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated bindings: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "A__TokShared") || !strings.Contains(text, "B__TokShared") {
		t.Fatalf("generated bindings missing namespaced imported token constants")
	}
	if !strings.Contains(text, "A__SharedNode") || !strings.Contains(text, "B__SharedNode") {
		t.Fatalf("generated bindings missing namespaced imported node constants")
	}
}

func adaptManifest(
	typed toolchain.SemanticManifest[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind],
	scopeMap map[dsl.LangSpecLexerTokenType]string,
) toolchain.SemanticManifest[string, string] {

	stringManifest := toolchain.SemanticManifest[string, string]{
		InvalidScope:    typed.InvalidScope,
		BaseTokenScopes: make(map[string]string),
		NodeBindings:    make(map[string]toolchain.NodeBinding[string]),
	}

	for tok, scope := range scopeMap {
		stringManifest.BaseTokenScopes[tok.String()] = scope
	}

	for kind, binding := range typed.NodeBindings {
		tokenScopes := make(map[string][]string)
		for tok, scopes := range binding.TokenScopes {
			tokenScopes[tok.String()] = scopes
		}

		stringManifest.NodeBindings[kind.String()] = toolchain.NodeBinding[string]{
			Scopes:      binding.Scopes,
			MetaScope:   binding.MetaScope,
			TokenScopes: tokenScopes,
		}
	}

	return stringManifest
}

func adaptGoOverrideProducer(
	compileResult *dsl.LangSpecCompileResult,
	compiler *dsl.LangSpecCompiler,
) func(*editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, toolchain.SublimeContext] {

	ruleset := dsl.LangSpecCompilerLexingRuleSet(compiler)
	editorRuleset := convertRulesetToEditorForToolchain(ruleset)
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	manifest := dsleditor.LangSpecEditorManifest
	manifest.BaseTokenScopes = scopeMap

	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind](manifest)

	nativeProducer := dsleditor.BuildEditorOverrideProducer(editorRuleset, ctxProducer)

	tokenMap := map[string]dsl.LangSpecLexerTokenType{
		dslspec.TokLineComment.String():  dslspec.TokLineComment,
		dslspec.TokBlockComment.String(): dslspec.TokBlockComment,
		dslspec.TokRegexLiteral.String(): dslspec.TokRegexLiteral,
		dslspec.TokIdentifier.String():   dslspec.TokIdentifier,
	}

	nodeMap := map[string]dsl.LangSpecParserNodeKind{
		dslspec.NodeParseSymbolReference.String():   dslspec.NodeParseSymbolReference,
		dslspec.NodeParseTemplateReference.String(): dslspec.NodeParseTemplateReference,
	}

	sym := compileResult.CompiledSymbols

	return func(ctx *editor.EditorCtx[rune, uint32, uint32, string, uint32]) []*editor.EditorOverride[rune, uint32, uint32, string, uint32, toolchain.SublimeContext] {
		var nativeToken *dsl.LangSpecLexerTokenType
		var nativeNode *dsl.LangSpecParserNodeKind

		if ctx.Token != nil && sym != nil {
			if tokEnum, ok := tokenMap[sym.TokenName(*ctx.Token)]; ok {
				nativeToken = &tokEnum
			}
		}
		if ctx.NodeKind != nil && sym != nil {
			if nodeEnum, ok := nodeMap[sym.NodeKindName(*ctx.NodeKind)]; ok {
				nativeNode = &nodeEnum
			}
		}

		if nativeToken == nil && nativeNode == nil {
			return nil
		}

		nativeCtx := &dsleditor.EditorCtx{Token: nativeToken, NodeKind: nativeNode}
		nativeOverrides := nativeProducer(nativeCtx)
		if len(nativeOverrides) == 0 {
			return nil
		}

		out := make([]*editor.EditorOverride[rune, uint32, uint32, string, uint32, toolchain.SublimeContext], len(nativeOverrides))
		for i, nativeOverride := range nativeOverrides {
			out[i] = &editor.EditorOverride[rune, uint32, uint32, string, uint32, toolchain.SublimeContext]{
				Pattern:          nativeOverride.Pattern,
				PatternRegex:     nativeOverride.PatternRegex,
				MatchContext:     nativeOverride.MatchContext,
				Captures:         nativeOverride.Captures,
				DelimitedPayload: nativeOverride.DelimitedPayload,
				ForeignPayload:   nativeOverride.ForeignPayload,
			}

			if nativeOverride.DelimitedPayload != nil {
				out[i].DelimitedPayload = &editor.DelimitedPayload[rune, toolchain.SublimeContext]{
					StateLabel:   nativeOverride.DelimitedPayload.StateLabel,
					BodyContext:  nativeOverride.DelimitedPayload.BodyContext,
					ClosePattern: nativeOverride.DelimitedPayload.ClosePattern,
					CloseContext: nativeOverride.DelimitedPayload.CloseContext,
				}
			}
			if nativeOverride.ForeignPayload != nil {
				out[i].ForeignPayload = &editor.ForeignMachinePayload[rune, toolchain.SublimeContext]{
					MachineID:      nativeOverride.ForeignPayload.MachineID,
					MachineContext: nativeOverride.ForeignPayload.MachineContext,
					EscapePattern:  nativeOverride.ForeignPayload.EscapePattern,
					EscapeCaptures: nativeOverride.ForeignPayload.EscapeCaptures,
				}
			}
		}

		return out
	}
}

func convertRulesetToEditorForToolchain(
	ruleset *langspec.LexerRuleset[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole],
) *editor.LexingRuleSet[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole] {
	sourceRules := langspec.LexerRulesetGetRules(*ruleset)
	out := make([]editor.LexingRule[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole], 0, len(sourceRules))
	for _, rule := range sourceRules {
		out = append(out, editor.LexingRule[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole]{
			Token:          rule.Token,
			Role:           rule.Role,
			Pattern:        rule.Pattern,
			Priority:       rule.Priority,
			LexerState:     dsl.LangSpecLexerStateInitial,
			StackKind:      rule.StackKind,
			StackTargets:   append([]string(nil), rule.StackStates...),
			StackPopAmount: rule.StackPopAmount,
		})
	}
	return editor.LexingRuleSetCreate(out...)
}
