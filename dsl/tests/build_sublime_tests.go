package tests

import (
	"langspec/dsl"
	"langspec/dsl/editor"
	"memcore"
	"memforge"
	"testing"
)

const testSublimeSyntaxFile = "assets/testing/lspec.sublime-syntax"

func TestBuildSublimeSyntaxForDSL(t *testing.T) {
	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)
		if newSize > uint64(memcore.GigaByte) {
			panic("too much memory for a test")
		}
		return newSize
	})
	defer memforge.DynamicLinearAllocatorDestroy(scratchAllocator)

	compilerConfig := dsl.LangSpecCompilerConfigurationCreate(
		func(sizeBytes, alignment uint64) memcore.MarkRaw {
			return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
		},
		nil,
	).WithDiagnosticSink(dsl.DefaultLangSpecDiagnosticSink())

	compiler := dsl.LangSpecCompilerCreate(compilerConfig)
	defer dsl.LangSpecCompilerDestroy(compiler)

	if err := editor.BuildSublimeSyntaxForDSL(compiler, testSublimeSyntaxFile); err != nil {
		t.Fatalf("Sublime Syntax generation failed with error: %s", err.Error())
	}
}
