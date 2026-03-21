// Package cliutil provides shared setup for LangSpec //go:generate binaries: scratch allocator,
// allocation function, and thin wrappers around bootstrap.RunGoBindingsFromSpecFile and
// bootstrap.RunToolchainsFromSpecFile.
//
// Host languages that use generated Token/Node types in Sublime manifests should use two separate
// main packages: one that only imports cliutil + bootstrap (bindings-only), and one that imports
// the host compiler package for WithSublimeToolchain options. A single main cannot import the host
// package for the bindings step without the generated bindings file already present.
package cliutil
