package editor

import (
	"autarch/pattern"
	"fmt"
	"reflect"
)

/*
Pattern is the observation pattern type used for token recognition in the editor IR.
It is an alias for the Regula AST from autarch/pattern.
*/
type Pattern[TObservation any] = pattern.RegulaAST[TObservation]

/*
TokenID uniquely identifies a token definition within an EditorIR.
Used in boundaries, valid paths, and global trivia.
*/
type TokenID uint32

/*
ContextID uniquely identifies an editor context (a grammar rule or nest boundary).
Derived from grammar ID and node path; used as the key for context lookup.
*/
type ContextID uint64

/*
PathElementType discriminates between a token step and a context-reference step in a valid path.
*/
type PathElementType uint8

const (
	ElementToken       PathElementType = iota // Step consumes a token
	ElementContextRef                         // Step enters a nested context
)

/*
PathElement is one step in a StrictSequence expected path: either a token to match or a context to enter.
*/
type PathElement struct {
	Type          PathElementType
	Token         TokenID
	TargetContext ContextID
}

/*
MetadataProvider returns optional keyed data from a metadata value.
Used by the IR to let consumers attach domain-specific data to tokens or contexts.
*/
type MetadataProvider[TMetadata any] func(metadata TMetadata, key string) (result any, ok bool)

/*
Annotation attaches optional metadata to a token definition, boundary, or sequence.
*/
type Annotation[TMeta any] struct {
	Metadata TMeta
}

/*
TokenDefinition describes a single token for the editor: ID, name, recognition pattern, priority, and optional annotation.
*/
type TokenDefinition[TObservation, TMeta any, TTokenKind comparable] struct {
	ID         TokenID
	Name       string
	Pattern    Pattern[TObservation]
	Priority   int
	Annotation Annotation[TMeta]
}

/*
Boundary describes a transition: when a given token is seen, optionally enter or exit a context and emit an annotation.
*/
type Boundary[TMeta any] struct {
	OnTrigger      TokenID
	IsExit         bool
	TargetContext  ContextID
	EmitAnnotation Annotation[TMeta]
}

/*
StrictSequence is one valid path through a context: a sequence of path elements (tokens and/or context refs) and optional annotation.
*/
type StrictSequence[TMeta any] struct {
	ExpectedPath       []PathElement
	SequenceAnnotation Annotation[TMeta]
}

/*
EditorContext represents one editing context (e.g. a grammar rule or nest): ID, name for debug, boundaries, valid paths, and final flag.
*/
type EditorContext[TMeta any] struct {
	ID         ContextID
	Name       string // Rule name or grammar ID for debug display
	Boundaries []Boundary[TMeta]
	ValidPaths []StrictSequence[TMeta]
	IsFinal    bool
}

/*
EditorIR is the intermediate representation for editor/IDE integration: tokens, global trivia, contexts, start context, and metadata providers.
Not constructed directly; use EditorIRCreate or EditorIRConstruct from grammar and lexical rules.
*/
type EditorIR[TObservation, TTokenMeta, TContextMeta any, TTokenKind comparable] struct {
	tokenMeta    MetadataProvider[TTokenMeta]
	contextMeta  MetadataProvider[TContextMeta]
	tokens       []TokenDefinition[TObservation, TTokenMeta, TTokenKind]
	globalTrivia []TokenID
	contexts     map[ContextID]*EditorContext[TContextMeta]
	start        ContextID
}

/*
EditorIRCreate builds an EditorIR from pre-built tokens, global trivia, contexts, start context, and metadata providers.
Use when you have already constructed the context map and token list; otherwise use EditorIRConstruct from a grammar package.
*/
func EditorIRCreate[TObservation, TTokenMeta, TContextMeta any, TTokenKind comparable](
	tokenMeta MetadataProvider[TTokenMeta],
	contextMeta MetadataProvider[TContextMeta],
	tokens []TokenDefinition[TObservation, TTokenMeta, TTokenKind],
	globalTrivia []TokenID,
	contexts map[ContextID]*EditorContext[TContextMeta],
	start ContextID,
) *EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind] {
	return &EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind]{
		tokenMeta:    tokenMeta,
		contextMeta:  contextMeta,
		tokens:       tokens,
		globalTrivia: globalTrivia,
		contexts:     contexts,
		start:        start,
	}
}

/*
ExtractTokenMetadata looks up a key in the token metadata using the IR's token metadata provider.
Returns the value typed as TRequest, or an error if the provider did not return that type.
*/
func ExtractTokenMetadata[TObservation, TTokenMeta, TContextMeta, TRequest any, TTokenKind comparable](
	editor *EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind],
	metadata TTokenMeta,
	key string,
) (result TRequest, ok bool, err error) {
	return extractCore[TTokenMeta, TRequest](editor.tokenMeta, metadata, key)
}

/*
ExtractContextMetadata looks up a key in the context metadata using the IR's context metadata provider.
Returns the value typed as TRequest, or an error if the provider did not return that type.
*/
func ExtractContextMetadata[TObservation, TTokenMeta, TContextMeta, TRequest any, TTokenKind comparable](
	editor *EditorIR[TObservation, TTokenMeta, TContextMeta, TTokenKind],
	metadata TContextMeta,
	key string,
) (result TRequest, ok bool, err error) {
	return extractCore[TContextMeta, TRequest](editor.contextMeta, metadata, key)
}

func extractCore[TMeta, TRequest any](
	provider MetadataProvider[TMeta],
	metadata TMeta,
	key string,
) (result TRequest, ok bool, err error) {
	var zero TRequest
	res, ok := provider(metadata, key)
	if !ok {
		return zero, ok, nil
	}
	typed, assertOk := res.(TRequest)
	if !assertOk {
		return zero, ok, fmt.Errorf("returned value does not match %T", reflect.TypeFor[TRequest]())
	}
	return typed, ok, nil
}
