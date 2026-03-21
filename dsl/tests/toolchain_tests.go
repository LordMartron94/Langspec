package tests

import (
	"langspec/bootstrap"
	"langspec/dsl"
	dsleditor "langspec/dsl/editor"
	dslspec "langspec/dsl/spec"
	"langspec/editor"
	"langspec/toolchain"
	"lexarch"
	"testing"
)

const testClientDSLFile = "assets/testing/generated_lspec_spec.lspec"

func TestClientDSLToolchain(t *testing.T) {
	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	compileResult, err := dsl.LangSpecCompilerCompile(compiler, testClientDSLFile)
	if err != nil {
		t.Fatalf("Client compilation failed with error: %s", err.Error())
	}

	var sublimePragma *dsl.ToolPragma
	for _, pragma := range compileResult.CompiledToolPragmas {
		if pragma.ToolName == toolchain.SublimeToolName {
			sublimePragma = &pragma
			break
		}
	}

	if sublimePragma == nil || sublimePragma.Settings[toolchain.SublimeEnableKey] != "true" {
		t.Skip("Sublime toolchain not enabled in spec pragma")
	}

	stringManifest := adaptManifest(dsleditor.LangSpecEditorManifest, dsl.LangSpecCompilerScopeMap(compiler))

	err = bootstrap.RunToolchainsFromCompileResult(
		compileResult,
		bootstrap.WithSublimeToolchain(
			stringManifest,
			func(
				_ *lexarch.LexingRuleset[rune, string, string],
				_ func(*editor.EditorCtx[rune, string, string, string, string]) toolchain.SublimeContext,
			) func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext] {
				return adaptGoOverrideProducer(compiler)
			},
			[]string{".lspec"},
			".lspec",
		),
	)
	if err != nil {
		t.Fatalf("Toolchains failed: %s", err.Error())
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
	compiler *dsl.LangSpecCompiler,
) func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext] {

	ruleset := dsl.LangSpecCompilerLexingRuleSet(compiler)
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	manifest := dsleditor.LangSpecEditorManifest
	manifest.BaseTokenScopes = scopeMap

	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind](manifest)

	nativeProducer := dsleditor.BuildEditorOverrideProducer(ruleset, ctxProducer)

	tokenMap := map[string]dsl.LangSpecLexerTokenType{
		dslspec.TokLineComment.String():  dslspec.TokLineComment,
		dslspec.TokBlockComment.String(): dslspec.TokBlockComment,
		dslspec.TokRegexLiteral.String(): dslspec.TokRegexLiteral,
		dslspec.TokIdentifier.String():   dslspec.TokIdentifier,
	}

	nodeMap := map[string]dsl.LangSpecParserNodeKind{
		dslspec.NodeParseSymbolReference.String(): dslspec.NodeParseSymbolReference,
	}

	return func(ctx *editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext] {
		var nativeToken *dsl.LangSpecLexerTokenType
		var nativeNode *dsl.LangSpecParserNodeKind

		if ctx.Token != nil {
			if tokEnum, ok := tokenMap[*ctx.Token]; ok {
				nativeToken = &tokEnum
			}
		}
		if ctx.NodeKind != nil {
			if nodeEnum, ok := nodeMap[*ctx.NodeKind]; ok {
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

		out := make([]*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], len(nativeOverrides))
		for i, nativeOverride := range nativeOverrides {
			out[i] = &editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]{
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
