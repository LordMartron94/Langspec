package cliutil

import (
	"fmt"
	"langspec/bootstrap"
	"langspec/dsl"
	"langspec/toolchain"
	"reflect"
	"strconv"
)

/*
TokenConstraint and NodeConstraint bound token/node key types from LangSpec go_bindings:
numeric IDs with or without String(), or string enums (e.g. gomod/gowork).
*/
type TokenConstraint interface {
	comparable
}

type NodeConstraint interface {
	comparable
}

// InMemorySublimeRunnerConfig drives toolchains for a host language whose Sublime manifest
// and optional override factory are defined in Go (not JSON configuration-path).
type InMemorySublimeRunnerConfig[T TokenConstraint, N NodeConstraint] struct {
	SpecPath        string
	Manifest        toolchain.SemanticManifest[T, N]
	OverrideFactory toolchain.SublimeInMemoryOverrideFactory[dsl.LangSpecParserNodeKind]
	FileExtensions  []string
	ScopeExtension  string
}

func manifestNodeKeyForInMemoryRunner[T comparable](k T) string {
	if s, ok := any(k).(fmt.Stringer); ok {
		return s.String()
	}
	v := reflect.ValueOf(k)
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10)
	}
	return fmt.Sprint(k)
}

func manifestTokenKeyForInMemoryRunner[T comparable](k T) string {
	v := reflect.ValueOf(k)
	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(v.Uint(), 10)
	}
	if s, ok := any(k).(fmt.Stringer); ok {
		return s.String()
	}
	return fmt.Sprint(k)
}

/*
AdaptInMemorySublimeManifest converts a typed semantic manifest to string keys for
toolchains (same mapping historically used by host generators).
*/
func AdaptInMemorySublimeManifest[T TokenConstraint, N NodeConstraint](in toolchain.SemanticManifest[T, N]) toolchain.SemanticManifest[string, string] {
	out := toolchain.SemanticManifest[string, string]{
		InvalidScope:    in.InvalidScope,
		BaseTokenScopes: make(map[string]string),
		NodeBindings:    make(map[string]toolchain.NodeBinding[string]),
	}

	for k, v := range in.BaseTokenScopes {
		out.BaseTokenScopes[manifestTokenKeyForInMemoryRunner(k)] = v
	}

	for k, v := range in.NodeBindings {
		binding := toolchain.NodeBinding[string]{
			Scopes:           v.Scopes,
			MetaScope:        v.MetaScope,
			ExcludePrototype: v.ExcludePrototype,
			TokenScopes:      make(map[string][]string),
		}
		for tk, tv := range v.TokenScopes {
			binding.TokenScopes[manifestTokenKeyForInMemoryRunner(tk)] = tv
		}
		out.NodeBindings[manifestNodeKeyForInMemoryRunner(k)] = binding
	}
	return out
}

/*
RunInMemorySublimeToolchains compiles the spec at cfg.SpecPath and runs PRAGMA toolchains with
an in-memory Sublime manifest (bootstrap.WithSublimeToolchain). Other enabled tools run normally.
*/
func RunInMemorySublimeToolchains[T TokenConstraint, N NodeConstraint](cfg InMemorySublimeRunnerConfig[T, N]) error {
	stringManifest := AdaptInMemorySublimeManifest(cfg.Manifest)
	return runInMemorySublimeToolchainsFromStringConfig(
		cfg.SpecPath,
		stringManifest,
		cfg.OverrideFactory,
		cfg.FileExtensions,
		cfg.ScopeExtension,
	)
}

func runInMemorySublimeToolchainsFromStringConfig(
	specPath string,
	stringManifest toolchain.SemanticManifest[string, string],
	overrideFactory toolchain.SublimeInMemoryOverrideFactory[dsl.LangSpecParserNodeKind],
	fileExtensions []string,
	scopeExtension string,
) error {
	g := NewGenerator(specPath)
	defer g.Close()

	opts := []bootstrap.Option[dsl.LangSpecParserNodeKind]{
		bootstrap.WithDiagnosticSink[dsl.LangSpecParserNodeKind](dsl.DefaultLangSpecDiagnosticSink()),
		bootstrap.WithSublimeToolchain(
			stringManifest,
			overrideFactory,
			fileExtensions,
			scopeExtension,
		),
	}
	return g.RunToolchains(opts...)
}
