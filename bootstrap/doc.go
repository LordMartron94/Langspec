// Package bootstrap orchestrates the other LangSpec subpackages so that creating a
// parser and optional editor assets from a .lspec file is a single call.
//
// Use CompileParserFromSpec(specFile, alloc, opts...) to: compile the DSL, run
// toolchains (e.g. Sublime syntax generation when enabled in PRAGMA), and return a
// LangParser. Options: WithDiagnosticSink for human-readable diagnostics;
// WithSublimeOverrides(overrideProducer) to supply token overrides (e.g. from an
// OverrideRegistry.Producer()) for the Sublime toolchain. The override producer is
// only used when the spec enables the Sublime tool in PRAGMA; otherwise it is ignored.
package bootstrap
