// Command langspec-toolchain compiles a .lspec and runs LangSpec toolchains (go bindings, Sublime).
// Intended for //go:generate and CI, not runtime parser creation.
package main

import (
	"flag"
	"fmt"
	"langspec/cliutil"
	"os"
)

func main() {
	specPath := flag.String("spec", "", "path to the .lspec file (required)")
	flag.Parse()

	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "langspec-toolchain: -spec is required")
		os.Exit(2)
	}

	g := cliutil.NewGenerator(*specPath)
	defer g.Close()

	if err := g.RunToolchains(); err != nil {
		fmt.Fprintf(os.Stderr, "langspec-toolchain: %v\n", err)
		os.Exit(1)
	}
}
