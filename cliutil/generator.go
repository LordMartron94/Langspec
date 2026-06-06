package cliutil

import (
	"langspec/bootstrap"
	"langspec/dsl"
	"memarch"
	"memcore"
	"memforge"
)

/*
Generator owns the scratch memforge allocator and allocation function used by LangSpec bootstrap
codegen entry points.
*/
type Generator struct {
	specPath string
	scratch  memcore.MarkRaw
	allocFn  memarch.AllocationFn
	closed   bool
}

/*
NewGenerator builds a dynamic linear allocator and binds it to specPath for subsequent Run* calls.
*/
func NewGenerator(specPath string) *Generator {
	scratch := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)
		if newSize > uint64(memcore.GigaByte) {
			panic("langspec/cliutil: allocator too large")
		}
		return newSize
	}, "langspec cliutil")

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

/*
RunToolchains executes the generation pipeline. Pass bootstrap.WithToolchainFilter
to restrict execution to specific outputs (e.g., just "go_bindings").
*/
func (g *Generator) RunToolchains(opts ...bootstrap.Option[dsl.LangSpecParserNodeKind]) error {
	if len(opts) == 0 {
		opts = []bootstrap.Option[dsl.LangSpecParserNodeKind]{
			bootstrap.WithDiagnosticSink[dsl.LangSpecParserNodeKind](dsl.DefaultLangSpecDiagnosticSink()),
		}
	}
	return bootstrap.RunToolchainsFromSpecFile[dsl.LangSpecParserNodeKind](g.specPath, g.allocFn, opts...)
}
