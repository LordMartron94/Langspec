package editor

import (
	"langspec/dsl"
	"memcore"
	"memforge"
	"testing"
)

const testLspecFile = "assets/testing/input.lspec"
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

	_, err := dsl.LangSpecCompilerCompile(compiler, testLspecFile)
	if err != nil {
		t.Fatalf("Compilation failed with error: %s", err.Error())
	}

	if err := BuildSublimeSyntaxForDSL(compiler, testSublimeSyntaxFile); err != nil {
		t.Fatalf("Sublime Syntax generation failed with error: %s", err.Error())
	}
}
