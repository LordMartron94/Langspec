package tests

import (
	"fmt"
	"langspec/dsl"
	"lexarch"
	"memcore"
	"memforge"
	"syntaxa"
)

// setupTestCompiler returns the compiler, alloc tools, and a deterministic teardown func.
func setupTestCompiler() (
	compiler *dsl.LangSpecCompiler,
	scratchAllocFn func(sizeBytes, alignment uint64) memcore.MarkRaw,
	diagnosticSink *dsl.LangSpecDiagnosticSink,
	teardownFn func(),
) {
	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)
		if newSize > uint64(memcore.GigaByte) {
			panic("too much memory for a test")
		}
		return newSize
	})

	scratchAllocFn = func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
	}

	sink := dsl.DefaultLangSpecDiagnosticSink()

	compilerConfig := dsl.LangSpecCompilerConfigurationCreate(
		scratchAllocFn,
		nil,
	).WithDiagnosticSink(sink)

	compiler = dsl.LangSpecCompilerCreate(compilerConfig)

	teardown := func() {
		dsl.LangSpecCompilerDestroy(compiler)
		memforge.DynamicLinearAllocatorDestroy(scratchAllocator)
	}

	return compiler, scratchAllocFn, sink, teardown
}

func debugParseResult(
	enable bool,
	sink *dsl.LangSpecDiagnosticSink,
	trace *syntaxa.ParseTrace[string],
	rootNode *syntaxa.SyntaxaLSTNode[rune, string, string, string],
) {
	if !enable {
		return
	}

	dsl.RenderParseTrace(sink.Writer, trace, func(t string) string { return t })

	if rootNode != nil {
		lstDump := rootNode.DebugDump(
			syntaxa.LSTDebugFormatter[rune, string, string, string]{
				FormatKind:      func(k string) string { return k },
				FormatToken:     func(l lexarch.Lexeme[rune, string, string]) string { return string(l.Raw) },
				FormatAttribute: func(k string, v any) string { return fmt.Sprintf("%s=%v", k, v) },
				ShowTokens:      true,
				ShowAttributes:  true,
				ShowByteSpan:    true,
				ShowLineSpan:    true,
				ShowNodeID:      true,
				ShowRevision:    false,
				SlotPrefix:      "@",
			},
		)
		dsl.RenderLSTDump(sink.Writer, lstDump)
	}
}
