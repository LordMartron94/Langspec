package editor

import (
	"autarch/pattern"
	"fmt"
	"reflect"
)

// ------------------------------------------------------------- TYPE ALIASES

type Pattern = pattern.RegulaAST[rune]

// ------------------------------------------------------------- TYPES

type TokenID uint32
type StateID uint64

/* MetadataProvider should provide the result if any from the metadata.*/
type MetadataProvider[TMetadata any] func(metadata TMetadata, key string) (result any, ok bool)

/* Annotation adds editor metadata to a construct. */
type Annotation[TMeta any, TKind comparable] struct {
	/* Kind encapsulates the kind of the construct. */
	Kind TKind

	/* Metadata can attach custom metadata into a construct, that clients can use to extract client-specific data. */
	Metadata TMeta
}

/* TokenDefinition defines a single token. */
type TokenDefinition[TMeta any, TTokenKind comparable] struct {
	ID         TokenID
	Name       string
	Pattern    Pattern
	Priority   int
	Annotation Annotation[TMeta, TTokenKind]
}

/* State encapsulates a single state of being for the language. */
type State[TTransitionMeta any, TTransitionKind comparable] struct {
	ID StateID

	Transitions []Transition[TTransitionMeta, TTransitionKind]
	IsFinal     bool
}

/* Transition defines the transition from one state to the other. */
type Transition[TMeta any, TKind comparable] struct {
	On TokenID
	To StateID

	EmitAnnotation Annotation[TMeta, TKind]
	StackOperation StackOperation
}

type StackOpKind int

const (
	PushOp StackOpKind = iota + 1
	PopOp
	SwapOp
	NoneOp
)

/* StackOperation defines a transition's stack interactions. */
type StackOperation struct {
	Kind StackOpKind

	Push StateID
	Pop  int
	Swap StateID
}

// ------------------------------------------------------------- EDITOR IR

/*
EditorIR encapsulates the tree structure that defines a LangSpec language for
use in editors.
*/
type EditorIR[TTokenDefinitionMeta, TTransitionMeta any, TTokenKind, TTransitionKind comparable] struct {
	tokenMeta      MetadataProvider[TTokenDefinitionMeta]
	transitionMeta MetadataProvider[TTransitionMeta]

	tokens []TokenDefinition[TTokenDefinitionMeta, TTokenKind]
	states map[StateID]*State[TTransitionMeta, TTransitionKind]
	start  StateID
}

/* EditorIRCreate creates an editor IR instance. */
func EditorIRCreate[TTokenDefinitionMeta, TTransitionMeta any, TTokenKind, TTransitionKind comparable](
	tokenMeta MetadataProvider[TTokenDefinitionMeta],
	transitionMeta MetadataProvider[TTransitionMeta],

	tokens []TokenDefinition[TTokenDefinitionMeta, TTokenKind],
	states map[StateID]*State[TTransitionMeta, TTransitionKind],
	start StateID,
) *EditorIR[TTokenDefinitionMeta, TTransitionMeta, TTokenKind, TTransitionKind] {
	return &EditorIR[TTokenDefinitionMeta, TTransitionMeta, TTokenKind, TTransitionKind]{
		tokenMeta:      tokenMeta,
		transitionMeta: transitionMeta,
		tokens:         tokens,
		states:         states,
		start:          start,
	}
}

/*
ExtractTokenMetadata extracts specific metadata from a token.

It returns a potential result, whether the key existed (ok), and a potential error.
*/
func ExtractTokenMetadata[TTokenDefinitionMeta, TTransitionMeta, TRequest any, TTokenKind, TTransitionKind comparable](
	editor *EditorIR[TTokenDefinitionMeta, TTransitionMeta, TTokenKind, TTransitionKind],
	metadata TTokenDefinitionMeta,
	key string,
) (
	result TRequest,
	ok bool,
	err error,
) {
	return extractCore[TTokenDefinitionMeta, TRequest](editor.tokenMeta, metadata, key)
}

/*
ExtractTransitionMetadata extracts specific metadata from a transition.

It returns a potential result, whether the key existed (ok), and a potential error.
*/
func ExtractTransitionMetadata[TTokenDefinitionMeta, TTransitionMeta, TRequest any, TTokenKind, TTransitionKind comparable](
	editor *EditorIR[TTokenDefinitionMeta, TTransitionMeta, TTokenKind, TTransitionKind],
	metadata TTransitionMeta,
	key string,
) (
	result TRequest,
	ok bool,
	err error,
) {
	return extractCore[TTransitionMeta, TRequest](editor.transitionMeta, metadata, key)
}

// ------------------------------------------------------------- PRIVATE HELPERS

func extractCore[TMeta, TRequest any](
	provider MetadataProvider[TMeta],
	metadata TMeta,
	key string,
) (
	result TRequest,
	ok bool,
	err error,
) {
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
