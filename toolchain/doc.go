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
package toolchain
