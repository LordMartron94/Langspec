// Package bootstrap orchestrates the other LangSpec subpackages so that creating a
// parser and optional editor assets from a .lspec file is a single call.
//
// Use CompileParserFromSpec(specFile, alloc, opts...) to compile the DSL, run
// toolchains (e.g. Sublime syntax generation when enabled in PRAGMA), and
// return a LangParser.
//
// Options: WithDiagnosticSink for human-readable diagnostics; WithSublimeToolchain
// to supply an in-memory manifest and override factory for the Sublime toolchain.
//
// # Sublime toolchain: JSON vs in-memory manifest
//
// The bootstrap supports two ways to drive the Sublime toolchain:
//
//  1. JSON manifest: If WithSublimeToolchain is not used, runToolchains calls
//     toolchain.RunSublimeToolchain(compileResult, nil). That reads PRAGMA
//     (enable, output-path, configuration-path) and loads the manifest from the
//     JSON file at configuration-path. Override producer is nil unless the caller
//     invokes the toolchain separately with one.
//
//  2. In-memory manifest: If WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension)
//     is used, runToolchains uses the supplied manifest and calls
//     toolchain.RunSublimeToolchainFromMemory. PRAGMA must still set enable = true
//     and output-path; configuration-path is ignored. The factory is called to
//     build the override producer used when generating the syntax file.
package bootstrap
