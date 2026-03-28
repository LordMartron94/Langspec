// Command langspec-toolchain compiles a .lspec and runs LangSpec toolchains (go bindings, Sublime).
// Intended for //go:generate and CI, not runtime parser creation.
package main

import (
	"flag"
	"fmt"
	"langspec/bootstrap"
	"langspec/cliutil"
	"langspec/dsl"
	"os"
	"strings"
)

func main() {
	specPath := flag.String("spec", "", "path to the .lspec file (required)")
	toolchains := flag.String("toolchains", "", "comma-separated toolchain names to run (default: all enabled in PRAGMA), e.g. go_bindings")
	flag.Parse()

	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "langspec-toolchain: -spec is required")
		os.Exit(2)
	}

	g := cliutil.NewGenerator(*specPath)
	defer g.Close()

	opts := []bootstrap.Option{bootstrap.WithDiagnosticSink(dsl.DefaultLangSpecDiagnosticSink())}
	if strings.TrimSpace(*toolchains) != "" {
		parts := strings.Split(*toolchains, ",")
		names := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				names = append(names, s)
			}
		}
		if len(names) > 0 {
			opts = append(opts, bootstrap.WithToolchainFilter(names...))
		}
	}

	if err := g.RunToolchains(opts...); err != nil {
		fmt.Fprintf(os.Stderr, "langspec-toolchain: %v\n", err)
		os.Exit(1)
	}
}
