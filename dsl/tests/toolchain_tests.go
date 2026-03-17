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

	// 1. Extract tool configuration from Pragmas to simulate the bootstrap pipeline
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

	outputPath := sublimePragma.Settings[toolchain.SublimeOutputPathKey]
	stringManifest := adaptManifest(dsleditor.LangSpecEditorManifest, dsl.LangSpecCompilerScopeMap(compiler))

	// 2. Use the MEMORY path properly
	// We pass the manifest, the adapter, and the metadata directly.
	err = toolchain.RunSublimeToolchainFromMemory(
		compileResult,
		stringManifest,
		adaptGoOverrideProducer(compiler),
		outputPath,
		[]string{".lspec"},
		".lspec",
	)

	if err != nil {
		t.Fatalf("Memory-based Sublime Toolchain failed: %s", err.Error())
	}

	// 3. Run Go Bindings
	err = toolchain.RunGoBindingsToolchain(compileResult)
	if err != nil {
		t.Fatalf("Go Binding Toolchain failed: %s", err.Error())
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

	// Map Base Token Scopes (using the compiler's dynamic scope map)
	for tok, scope := range scopeMap {
		stringManifest.BaseTokenScopes[tok.String()] = scope
	}

	// Map Node Bindings
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

	// Recreate native context producer
	manifest := dsleditor.LangSpecEditorManifest
	manifest.BaseTokenScopes = scopeMap

	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind](manifest)

	// Build the native producer WITH dependencies
	nativeProducer := dsleditor.BuildEditorOverrideProducer(ruleset, ctxProducer)

	tokenMap := map[string]dsl.LangSpecLexerTokenType{
		dsl.TokLineComment.String():  dsl.TokLineComment,
		dsl.TokBlockComment.String(): dsl.TokBlockComment,
		dsl.TokRegexLiteral.String(): dsl.TokRegexLiteral,
		dsl.TokIdentifier.String():   dsl.TokIdentifier,
	}

	nodeMap := map[string]dsl.LangSpecParserNodeKind{
		dsl.NodeParseSymbolReference.String(): dsl.NodeParseSymbolReference,
	}

	// Return slice of VALUES
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
