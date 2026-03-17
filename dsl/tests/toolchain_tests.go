package tests

import (
	"langspec/dsl"
	dsleditor "langspec/dsl/editor"
	"langspec/editor"
	"langspec/toolchain"
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

	// 2. Dispatch to the generic JSON toolchain, passing the adapted Go Overrides.
	// We now pass the compiler so the adapter can fulfill dependencies.
	err = toolchain.RunSublimeToolchain(compileResult, adaptGoOverrideProducer(compiler))

	if err != nil {
		t.Fatalf("Dynamic Sublime Toolchain failed: %s", err.Error())
	}
}

// adaptGoOverrideProducer bridges the string-based toolchain to your strongly-typed Go logic.
func adaptGoOverrideProducer(compiler *dsl.LangSpecCompiler) func(*editor.EditorCtx[rune, string, string, string, string]) []*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext] {

	// 1. Fulfill the new dependencies
	ruleset := dsl.LangSpecCompilerLexingRuleSet(compiler)
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	// Recreate the native context producer for the dynamic overrides
	ctxCfg := toolchain.ContextProducerConfig[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind]{
		InvalidScope: "invalid.illegal.unexpected-token",
		GetBaseScope: func(token dsl.LangSpecLexerTokenType) string { return scopeMap[token] },
		GetNodeScope: func(nodeKind dsl.LangSpecParserNodeKind, token *dsl.LangSpecLexerTokenType) string {
			binding, ok := dsleditor.GetGeneratorBindings()[nodeKind] // Assuming this is exposed
			if !ok {
				return ""
			}
			if token != nil && len(binding.TokenScopes) > 0 {
				if ts, ok := binding.TokenScopes[*token]; ok && len(ts) > 0 {
					return ts[0]
				}
			}
			if len(binding.Scopes) > 0 {
				return binding.Scopes[0]
			}
			return ""
		},
	}
	ctxProducer := toolchain.BuildContextProducer[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState](ctxCfg)

	// Build the native producer WITH dependencies
	nativeProducer := dsleditor.BuildEditorOverrideProducer(ruleset, ctxProducer)

	// 2. Map known string representations back to your Enums (Include Identifiers!)
	tokenMap := map[string]dsl.LangSpecLexerTokenType{
		dsl.TokLineComment.String():  dsl.TokLineComment,
		dsl.TokBlockComment.String(): dsl.TokBlockComment,
		dsl.TokRegexLiteral.String(): dsl.TokRegexLiteral,
		dsl.TokIdentifier.String():   dsl.TokIdentifier,
	}

	nodeMap := map[string]dsl.LangSpecParserNodeKind{
		dsl.NodeParseSymbolReference.String(): dsl.NodeParseSymbolReference,
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

		// If it maps to nothing we care about, exit early
		if nativeToken == nil && nativeNode == nil {
			return nil
		}

		// 3. Query the native producer
		nativeCtx := &dsleditor.EditorCtx{Token: nativeToken, NodeKind: nativeNode}
		nativeOverrides := nativeProducer(nativeCtx)
		if len(nativeOverrides) == 0 {
			return nil
		}

		// 4. Map the results back to strings, ensuring NO DATA IS DROPPED
		out := make([]*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], len(nativeOverrides))
		for i, nativeOverride := range nativeOverrides {

			// Dereference nativeOverride if your nativeProducer returns pointers
			adaptedOverride := editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]{
				Pattern:      nativeOverride.Pattern,
				PatternRegex: nativeOverride.PatternRegex,
				MatchContext: nativeOverride.MatchContext,
				Captures:     nativeOverride.Captures,
			}

			if nativeOverride.DelimitedPayload != nil {
				adaptedOverride.DelimitedPayload = &editor.DelimitedPayload[rune, toolchain.SublimeContext]{
					StateLabel:   nativeOverride.DelimitedPayload.StateLabel,
					BodyContext:  nativeOverride.DelimitedPayload.BodyContext,
					ClosePattern: nativeOverride.DelimitedPayload.ClosePattern,
					CloseContext: nativeOverride.DelimitedPayload.CloseContext,
				}
			}

			if nativeOverride.ForeignPayload != nil {
				adaptedOverride.ForeignPayload = &editor.ForeignMachinePayload[rune, toolchain.SublimeContext]{
					MachineID:      nativeOverride.ForeignPayload.MachineID,
					MachineContext: nativeOverride.ForeignPayload.MachineContext,
					EscapePattern:  nativeOverride.ForeignPayload.EscapePattern,
					EscapeCaptures: nativeOverride.ForeignPayload.EscapeCaptures,
				}
			}

			out[i] = &adaptedOverride
		}

		return out
	}
}
