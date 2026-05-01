// Command langspec-toolchain compiles a .lspec and runs LangSpec toolchains (go bindings, Sublime, tm_comments).
// Intended for //go:generate and CI, not runtime parser creation.
// Optional -sublime-json-config supplies an external Sublime JSON manifest using the in-memory bootstrap path
// when PRAGMA enables tool.sublime but omits configuration-path.
package main

import (
	"flag"
	"fmt"
	"langspec/bootstrap"
	"langspec/cliutil"
	"langspec/dsl"
	"langspec/toolchain"
	"os"
	"strings"
)

func main() {
	specPath := flag.String("spec", "", "path to the .lspec file (required)")
	toolchains := flag.String("toolchains", "", "comma-separated toolchain names to run (default: all enabled in PRAGMA), e.g. go_bindings,sublime,tm_comments")
	sublimeJSONConfig := flag.String("sublime-json-config", "", "path to Sublime JSON (scope_manifest, file_extensions, scope_extension); uses bootstrap in-memory Sublime path so PRAGMA tool.sublime does not need configuration-path (enable and output-path still required)")
	flag.Parse()

	if *specPath == "" {
		fmt.Fprintln(os.Stderr, "langspec-toolchain: -spec is required")
		os.Exit(2)
	}

	g := cliutil.NewGenerator(*specPath)
	defer g.Close()

	opts := []bootstrap.Option[dsl.LangSpecParserNodeKind]{
		bootstrap.WithDiagnosticSink[dsl.LangSpecParserNodeKind](dsl.DefaultLangSpecDiagnosticSink()),
	}
	if strings.TrimSpace(*toolchains) != "" {
		parts := strings.Split(*toolchains, ",")
		names := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				names = append(names, s)
			}
		}
		if len(names) > 0 {
			opts = append(opts, bootstrap.WithToolchainFilter[dsl.LangSpecParserNodeKind](names...))
		}
	}

	if p := strings.TrimSpace(*sublimeJSONConfig); p != "" {
		sublimeCfg, err := toolchain.LoadSublimeConfigFromJSON(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "langspec-toolchain: -sublime-json-config: %v\n", err)
			os.Exit(1)
		}
		opts = append(opts, bootstrap.WithSublimeToolchain[dsl.LangSpecParserNodeKind](
			sublimeCfg.Manifest,
			nil,
			sublimeCfg.FileExtensions,
			sublimeCfg.ScopeExtension,
		))
	}

	if err := g.RunToolchains(opts...); err != nil {
		fmt.Fprintf(os.Stderr, "langspec-toolchain: %v\n", err)
		os.Exit(1)
	}
}
