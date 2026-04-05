// Package bootstrap wires LangSpec DSL compilation to a runtime LangParser and exposes
// separate entry points for codegen toolchains (go bindings, Sublime syntax).
//
// # Runtime: parser from a spec
//
// Use CompileParserFromSpec(specFile, alloc, opts...) to compile the DSL and return
// a LangParser. Use CompileParserFromSpecWithCompiledSymbols when you also need the
// target language CompiledSymbolTable (e.g. resolving node-kind IDs in LST dumps).
// This does not run toolchains (no generated files). Options:
// WithDiagnosticSink for human-readable compile diagnostics; WithNodePoolPrefill and
// WithNodePoolGrowFn for Syntaxa LST node pool tuning (see langspec.LangParserConfiguration).
// WithSublimeToolchain is not used by CompileParserFromSpec (ignored); use RunToolchainsFromSpecFile for that.
//
// # Codegen: toolchains
//
// Use RunGoBindingsFromSpecFile(specFile, alloc, opts...) to emit go_bindings only (first
// codegen step when generated Token/Node types must exist before the rest of the package
// compiles). Use RunToolchainsFromSpecFile(specFile, alloc, opts...) to run go_bindings, Sublime,
// and tm_comments per PRAGMA. Same options as RunToolchainsFromCompileResult for the full pipeline,
// including WithSublimeToolchain for an in-memory manifest and override factory, and
// WithTMCommentsToolchain to override TM comment settings from Go.
//
// Use RunToolchainsFromCompileResult when you already have a LangSpecCompileResult
// (e.g. after dsl.LangSpecCompilerCompile).
//
// # Sublime toolchain: JSON vs in-memory manifest
//
// When RunToolchainsFromSpecFile / RunToolchainsFromCompileResult runs without
// WithSublimeToolchain, toolchain.RunSublimeToolchain loads the manifest from the JSON
// file at PRAGMA configuration-path (enable, output-path, configuration-path).
//
// With WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension), the
// in-memory manifest is used and configuration-path is ignored; factory may be nil.
//
// Toolchains also run the Go bindings generator when tool.go_bindings is enabled in PRAGMA,
// and RunTMCommentsToolchain when tool.tm_comments is enabled (or WithTMCommentsToolchain supplies config).
// Use WithToolchainFilter("go_bindings", "sublime", "tm_comments", ...) to restrict which run.
package bootstrap
