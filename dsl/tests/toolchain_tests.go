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

	// 2. Dispatch to the generic JSON toolchain, passing the adapted Go Overrides
	err = toolchain.RunSublimeToolchain(compileResult, adaptGoOverrideProducer())

	if err != nil {
		t.Fatalf("Dynamic Sublime Toolchain failed: %s", err.Error())
	}
}

// adaptGoOverrideProducer bridges the string-based toolchain to your strongly-typed Go logic.
func adaptGoOverrideProducer() func(*editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool) {
	nativeProducer := dsleditor.BuildEditorOverrideProducer()

	// Map known string representations back to your Enums
	tokenMap := map[string]dsl.LangSpecLexerTokenType{
		dsl.TokLineComment.String():  dsl.TokLineComment,
		dsl.TokBlockComment.String(): dsl.TokBlockComment,
		dsl.TokRegexLiteral.String(): dsl.TokRegexLiteral,
	}

	return func(ctx *editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool) {
		if ctx.Token == nil {
			return nil, false
		}

		tokEnum, ok := tokenMap[*ctx.Token]
		if !ok {
			return nil, false
		}

		// 1. Query the native producer
		nativeCtx := &dsleditor.EditorCtx{Token: &tokEnum}
		nativeOverride, has := nativeProducer(nativeCtx)
		if !has || nativeOverride == nil {
			return nil, false
		}

		// 2. Map the heavily-typed EditorOverride to the string-typed EditorOverride
		out := &editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext]{
			Pattern:      nativeOverride.Pattern,
			MatchContext: nativeOverride.MatchContext,
			Captures:     nativeOverride.Captures,
		}

		if nativeOverride.DelimitedPayload != nil {
			out.DelimitedPayload = &editor.DelimitedPayload[rune, toolchain.SublimeContext]{
				StateLabel:   nativeOverride.DelimitedPayload.StateLabel,
				BodyContext:  nativeOverride.DelimitedPayload.BodyContext,
				ClosePattern: nativeOverride.DelimitedPayload.ClosePattern,
				CloseContext: nativeOverride.DelimitedPayload.CloseContext,
			}
		}

		if nativeOverride.ForeignPayload != nil {
			out.ForeignPayload = &editor.ForeignMachinePayload[rune, toolchain.SublimeContext]{
				MachineID:      nativeOverride.ForeignPayload.MachineID,
				MachineContext: nativeOverride.ForeignPayload.MachineContext,
				EscapePattern:  nativeOverride.ForeignPayload.EscapePattern,
				EscapeCaptures: nativeOverride.ForeignPayload.EscapeCaptures,
			}
		}

		return out, true
	}
}
