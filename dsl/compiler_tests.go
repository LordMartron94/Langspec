package dsl

import (
	"fmt"
	"foundation/system"
	"io"
	"langspec"
	"lexarch"
	"memarch"
	"memcore"
	"memforge"
	"os"
	"testing"
)

const TestFile = "assets/testing/input.lspec"

func TestDSLCompiler(t *testing.T) {
	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)

		if newSize > uint64(memcore.GigaByte) {
			panic("too much memory for a test")
		}

		return newSize
	})
	defer memforge.DynamicLinearAllocatorDestroy(scratchAllocator)

	scratchAllocFn := func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
	}
	compilerConfig := LangSpecCompilerConfigurationCreate(
		scratchAllocFn,
		nil,
	).WithDiagnosticSink(DefaultLangSpecDiagnosticSink())

	compiler := LangSpecCompilerCreate(compilerConfig)
	defer LangSpecCompilerDestroy(compiler)

	LangSpecCompilerDebugGrammar(compiler)

	result, err := LangSpecCompilerCompile(compiler, TestFile)

	LangSpecCompilerDebugResult(compiler, result, &CompilerDebugConfig{
		DebugParseTrace: false,
		DebugLST:        true,
	})

	if err != nil {
		t.Fatalf("Compilation failed with error: %s", err.Error())
	}

	// testLex(t, result, scratchAllocFn, TestFile)
}

func testLex(
	t *testing.T,
	result *LangSpecCompileResult,
	scratchAllocationFn memarch.AllocationFn,
	sourceFile string,
) {
	lexer := langspec.LangParserLexerCreateFromSpec(
		scratchAllocationFn,
		memcore.GigaByte,
		result.CompiledLexerSpec,
	)
	defer lexarch.LexerClose(lexer)

	runes, err := system.FileReadAllRunes(sourceFile)
	if err != nil {
		t.Fatalf("read source file: %v", err)
	}

	session := lexarch.LexerSessionCreate[rune, string, string](
		"default",
		runes,
		lexarch.NewlineDetectorRune(),
		lexarch.ColumnAdvanceRune(4),
	)

	lexemes := make([]lexarch.Lexeme[rune, string, string], 0)
	for {
		lex := lexarch.LexerConsume(lexer, session)
		lexemes = append(lexemes, lex)
		if lex.Token == result.EOFToken {
			break
		}
	}

	renderCompiledSpecLexemes(os.Stdout, lexemes)
}

func renderCompiledSpecLexemes(w io.Writer, lexemes []lexarch.Lexeme[rune, string, string]) {
	if w == nil || len(lexemes) == 0 {
		return
	}
	for i, lexeme := range lexemes {
		debug := lexeme.DebugString(func(t string) string { return t }, func(r string) string { return r })
		fmt.Fprintf(w, "%05d) %s\n", i, debug)
	}
}
