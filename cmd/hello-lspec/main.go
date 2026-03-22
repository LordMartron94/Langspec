// Command hello-lspec compiles examples/hello_world.lspec and parses sample input (default "hello").
// Run from the ruleforge repository root: go run ./libs/langspec/cmd/hello-lspec
package main

import (
	"flag"
	"fmt"
	"os"

	"langspec"
	"langspec/bootstrap"
	"langspec/dsl"
	"memarch"
	"memcore"
	"memforge"
)

func main() {
	specPath := flag.String("spec", "libs/langspec/examples/hello_world.lspec", "path to the .lspec file")
	text := flag.String("text", "hello", "source text to parse (lowercase letters only for the stock hello spec)")
	verbose := flag.Bool("v", false, "print compiler diagnostics and validation to stdout")
	flag.Parse()

	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)
		if newSize > uint64(memcore.GigaByte) {
			panic("allocator cap exceeded")
		}
		return newSize
	})
	defer memforge.DynamicLinearAllocatorDestroy(scratchAllocator)

	scratchAllocFn := memarch.AllocationFn(func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
	})

	var opts []bootstrap.Option
	if *verbose {
		opts = append(opts, bootstrap.WithDiagnosticSink(dsl.DefaultLangSpecDiagnosticSink()))
	}
	parser, err := bootstrap.CompileParserFromSpec(*specPath, scratchAllocFn, opts...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "compile .lspec: %v\n", err)
		os.Exit(1)
	}
	defer langspec.LangParserDestroy(parser)

	tmp, err := os.CreateTemp("", "hello-lspec-*.txt")
	if err != nil {
		fmt.Fprintf(os.Stderr, "temp file: %v\n", err)
		os.Exit(1)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(*text); err != nil {
		tmp.Close()
		fmt.Fprintf(os.Stderr, "write temp: %v\n", err)
		os.Exit(1)
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "close temp: %v\n", err)
		os.Exit(1)
	}

	session := langspec.LangParserSessionCreate[rune](tmpPath, nil, false)
	_, root, syntaxErrors, err := langspec.LangParserParseFile(parser, session)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		fmt.Fprintf(os.Stderr, "syntax errors: %d\n", len(syntaxErrors.Errors))
		os.Exit(1)
	}
	if root == nil {
		fmt.Fprintln(os.Stderr, "parse: empty LST")
		os.Exit(1)
	}

	fmt.Println("Parsed OK. Root node kind:", root.Kind())
}
