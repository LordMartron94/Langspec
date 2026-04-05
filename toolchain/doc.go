// Package toolchain provides tool-specific helpers for LangSpec, including the
// Sublime Text syntax generation pipeline.
//
// # Sublime toolchain: manifest sources
//
// The Sublime toolchain can be driven in two ways:
//
//  1. JSON manifest (configuration-path): PRAGMA specifies configuration-path
//     pointing to a JSON file. RunSublimeToolchain reads that path, loads the
//     manifest via LoadSublimeConfigFromJSON, and runs the pipeline. Used when
//     the host does not supply an in-memory manifest (e.g. LangSpec compiling
//     itself, or users who rely only on JSON).
//
//  2. In-memory manifest: The host supplies a SemanticManifest and related
//     options (file extensions, scope extension, override factory) via
//     bootstrap.WithSublimeToolchain. The bootstrap then calls
//     RunSublimeToolchainFromMemory, which bypasses JSON loading. PRAGMA still
//     provides enable and output-path; configuration-path is ignored.
//
// Both paths converge in executeSublimeToolchain, which builds the editor IR
// and invokes the Sublime generator. BuildContextProducerFromManifest turns a
// SemanticManifest into a context producer for the editor IR.
//
// # Go bindings toolchain
//
// RunGoBindingsToolchain generates a Go file containing type Token and type Node
// (string-based) plus one const per token and per grammar node from the compiled
// spec. Enable it via PRAGMA tool.go_bindings with output-path and package-name.
// It does not run during parser creation: call it from a codegen path such as
// bootstrap.RunToolchainsFromSpecFile, bootstrap.RunGoBindingsFromSpecFile, or
// bootstrap.RunToolchainsFromCompileResult when tool.go_bindings is enabled (see
// package bootstrap). Use the generated types for type-safe references to tokens
// and nodes in downstream compilers and tooling.
//
// # TM comments toolchain (Sublime Text .tmPreferences)
//
// RunTMCommentsToolchain writes a plist-style .tmPreferences file so Sublime’s
// Toggle Comment uses the correct TM_COMMENT_* shell variables for your grammar’s scope.
// Enable it via PRAGMA tool.tm_comments with enable, output-path, exactly one of
// scope or scope-extension (same “source” + suffix rule as Sublime BaseScope),
// single-line-comment-start, and optionally block-comment-start / block-comment-end together.
//
// Templates are embedded in the package (go:embed); consumers do not ship separate template files.
// bootstrap.RunToolchainsFromSpecFile and RunToolchainsFromCompileResult run this step when
// tool.tm_comments is enabled, unless bootstrap.WithToolchainFilter omits "tm_comments".
// Use bootstrap.WithTMCommentsToolchain to supply TMCommentsConfiguration from Go instead of PRAGMA-derived fields.
package toolchain
