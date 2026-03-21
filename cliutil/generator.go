package cliutil

import (
	"langspec/bootstrap"
	"langspec/dsl"
	"memarch"
	"memcore"
	"memforge"
)

/* Generator owns the scratch memforge allocator and allocation function used by LangSpec bootstrap
codegen entry points. Typical pattern:

	g := cliutil.NewGenerator("lang.lspec")
	defer g.Close()
	_ = g.RunGoBindingsOnly()
*/
type Generator struct {
	specPath string
	scratch  memcore.MarkRaw
	allocFn  memarch.AllocationFn
	closed   bool
}

/* NewGenerator builds a dynamic linear allocator and binds it to specPath for subsequent Run* calls. */
func NewGenerator(specPath string) *Generator {
	scratch := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)
		if newSize > uint64(memcore.GigaByte) {
			panic("langspec/cliutil: allocator too large")
		}
		return newSize
	})

	allocFn := func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratch, sizeBytes, alignment)
	}

	return &Generator{
		specPath: specPath,
		scratch:  scratch,
		allocFn:  allocFn,
	}
}

/* SpecPath returns the path passed to NewGenerator. */
func (g *Generator) SpecPath() string {
	return g.specPath
}

/* AllocationFn exposes the allocator for hosts that need to pass it elsewhere (e.g. tests). */
func (g *Generator) AllocationFn() memarch.AllocationFn {
	return g.allocFn
}

/* Close destroys the scratch allocator. Safe to call more than once. */
func (g *Generator) Close() {
	if g.closed {
		return
	}
	memforge.DynamicLinearAllocatorDestroy(g.scratch)
	g.closed = true
}

/* RunGoBindingsOnly runs bootstrap.RunGoBindingsFromSpecFile with the default LangSpec diagnostic sink. */
func (g *Generator) RunGoBindingsOnly() error {
	sink := dsl.DefaultLangSpecDiagnosticSink()
	return bootstrap.RunGoBindingsFromSpecFile(g.specPath, g.allocFn, bootstrap.WithDiagnosticSink(sink))
}

/* RunToolchains runs bootstrap.RunToolchainsFromSpecFile. If opts is empty, only WithDiagnosticSink(default)
is applied (JSON Sublime or PRAGMA-only toolchains). Pass bootstrap.WithSublimeToolchain and other options
from your language package for in-memory Sublime + overrides.
*/
func (g *Generator) RunToolchains(opts ...bootstrap.Option) error {
	if len(opts) == 0 {
		opts = []bootstrap.Option{bootstrap.WithDiagnosticSink(dsl.DefaultLangSpecDiagnosticSink())}
	}
	return bootstrap.RunToolchainsFromSpecFile(g.specPath, g.allocFn, opts...)
}
