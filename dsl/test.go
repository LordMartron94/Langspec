package dsl

import (
	"memcore"
	"memforge"
	"testing"
)

const TestFile = "assets/testing/input.lspec"
const TestSyntaxFile = "assets/testing/lspec.sublime-syntax"

func TestDSLCompiler(t *testing.T) {
	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)

		if newSize > uint64(memcore.GigaByte) {
			panic("too much memory for a test")
		}

		return newSize
	})
	defer memforge.DynamicLinearAllocatorDestroy(scratchAllocator)

	compilerConfig := LangSpecCompilerConfigurationCreate(
		func(sizeBytes, alignment uint64) memcore.MarkRaw {
			return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
		},
		nil,
	)

	compiler := LangSpecCompilerCreate(compilerConfig)
	defer LangSpecCompilerDestroy(compiler)

	if err := LangSpecCompilerCompile(compiler, TestFile); err != nil {
		t.Fatalf("Compilation failed with error: %s", err.Error())
	}

	if err := LangSpecCompilerBuildSublimeSyntax(compiler, TestSyntaxFile); err != nil {
		t.Fatalf("Sublime Syntax generation failed with error: %s", err.Error())
	}
}
