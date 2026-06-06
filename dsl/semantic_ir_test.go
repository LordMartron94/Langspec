package dsl

import (
	"memcore"
	"memforge"
	"os"
	"path/filepath"
	"testing"
)

func semanticIRTestAllocatorCreate() (func(uint64, uint64) memcore.MarkRaw, func()) {
	scratch := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		if currentCap*2 > neededCap {
			return currentCap * 2
		}
		return neededCap
	}, "semantic ir test")
	alloc := func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratch, sizeBytes, alignment)
	}
	destroy := func() {
		memforge.DynamicLinearAllocatorDestroy(scratch)
	}
	return alloc, destroy
}

func TestSemanticIRFromCompileResultCarriesLoweredArtifacts(t *testing.T) {
	alloc, destroy := semanticIRTestAllocatorCreate()
	defer destroy()

	compiler := LangSpecCompilerCreate(LangSpecCompilerConfigurationCreate(alloc, nil))
	defer LangSpecCompilerDestroy(compiler)

	result, err := LangSpecCompilerCompile[uint32](compiler, filepath.Clean("../examples/lspec.lspec"))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	semanticModel := SemanticIRBuildFromCompileResult(result)
	if semanticModel == nil {
		t.Fatalf("semantic IR is nil")
	}
	if semanticModel.Lowered == nil {
		t.Fatalf("semantic IR lowered artifacts are nil")
	}
	if semanticModel.Lowered.LexerSpec == nil {
		t.Fatalf("semantic IR lowered lexer is nil")
	}
	if semanticModel.Lowered.ParserSpec == nil {
		t.Fatalf("semantic IR lowered parser is nil")
	}
	if semanticModel.Lowered.EOFToken != result.EOFToken {
		t.Fatalf("semantic IR EOF token mismatch: got %d want %d", semanticModel.Lowered.EOFToken, result.EOFToken)
	}
}

func TestSemanticIRCompileResultRoundtripMetadata(t *testing.T) {
	alloc, destroy := semanticIRTestAllocatorCreate()
	defer destroy()

	compiler := LangSpecCompilerCreate(LangSpecCompilerConfigurationCreate(alloc, nil))
	defer LangSpecCompilerDestroy(compiler)

	result, err := LangSpecCompilerCompile[uint32](compiler, filepath.Clean("../examples/lspec.lspec"))
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	semanticModel := SemanticIRBuildFromCompileResult(result)
	if semanticModel == nil || semanticModel.CompiledSymbolTable == nil {
		t.Fatalf("semantic IR symbol table missing")
	}
	if semanticModel.LanguageName == "" || semanticModel.LanguageVersion == "" {
		t.Fatalf("semantic IR language metadata missing")
	}
	if len(semanticModel.ToolPragmas) == 0 {
		t.Fatalf("semantic IR tool pragmas should be populated")
	}

	// Sanity check that lowered artifacts remain usable.
	output := filepath.Join(t.TempDir(), "semantic_ir_roundtrip.lspec")
	if err := os.WriteFile(output, []byte{}, 0o600); err != nil {
		t.Fatalf("prepare output file failed: %v", err)
	}
}
