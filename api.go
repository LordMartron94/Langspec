package langspec

import (
	"cmp"
	"fmt"
	"foundation/system"
	"lexarch"
	"memarch"
	"memcore"
	"syntaxa"
)

// ------------------------------------------------------------ LEXER SPEC

/* LexerSpec defines the complete lexing protocol for a language. */
type LexerSpec[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState comparable] struct {
	rulesets map[TLexerState]lexarch.LexingRuleset[TObservation, TToken, TTokenRole]

	initialState  TLexerState
	newlineDetect lexarch.NewlineDetector[TObservation]

	errorToken TToken
	eofToken   TToken
}

/* LexerSpecCreate constructs a lexer specification. */
func LexerSpecCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState comparable](
	errorToken, eofToken TToken,
	initialState TLexerState,
	newlineDetect lexarch.NewlineDetector[TObservation],
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	return &LexerSpec[TObservation, TToken, TTokenRole, TLexerState]{
		rulesets:      make(map[TLexerState]lexarch.LexingRuleset[TObservation, TToken, TTokenRole]),
		initialState:  initialState,
		newlineDetect: newlineDetect,
		errorToken:    errorToken,
		eofToken:      eofToken,
	}
}

/* WithRuleset registers a ruleset for a lexer state. */
func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) WithRuleset(
	state TLexerState,
	rules lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	l.rulesets[state] = rules
	return l
}

// ------------------------------------------------------------ PARSER SPEC

/* ParserSpec defines the grammar and AST contract. */
type ParserSpec[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
] struct {
	ruleSelector  syntaxa.RuleSelector[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]
	rootNodeKind  TNodeKind
	errorNodeKind TNodeKind

	freezeAfterParse bool
}

/* ParserSpecCreate constructs a parser specification. */
func ParserSpecCreate[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
](
	rootNodeKind, errorNodeKind TNodeKind,
	selector syntaxa.RuleSelector[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	freezeAfterParse bool,
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	return &ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		ruleSelector:     selector,
		rootNodeKind:     rootNodeKind,
		errorNodeKind:    errorNodeKind,
		freezeAfterParse: freezeAfterParse,
	}
}

// ------------------------------------------------------------ LANGUAGE SPEC

/* LangSpec ties lexer and parser semantics together. */
type LangSpec[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
] struct {
	Lexer  *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]
	Parser *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]
}

/* LangSpecCreate creates a language specification bundle. */
func LangSpecCreate[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
](
	lexer *LexerSpec[TObservation, TToken, TTokenRole, TLexerState],
	parser *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
) *LangSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	return &LangSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		Lexer:  lexer,
		Parser: parser,
	}
}

// ============================================================
// STREAMING CONFIGURATION
// ============================================================

/*
StreamingConfig defines the operational parameters for streaming lexing.

Streaming lexing incrementally feeds observations into the lexer rather than
loading the full source into memory. This enables parsing of very large files
or continuous streams with bounded memory usage.

Fields:

  - ReadChunkSize:
    Number of observations requested from the producer per read operation.
    Larger values increase throughput but raise peak buffering and latency.
    Smaller values reduce memory footprint but increase call overhead.

  - MaxBuffered:
    Maximum number of observations retained internally by the streaming
    session before backpressure is applied.

    This acts as a safety bound on memory usage and ensures the lexer cannot
    outpace the parser indefinitely.

Performance characteristics:

  - Memory: O(MaxBuffered)
  - Throughput: proportional to ReadChunkSize

Typical tuning strategy:

  - Increase ReadChunkSize for disk-heavy workloads
  - Decrease MaxBuffered for tight memory environments
*/
type StreamingConfig struct {
	ReadChunkSize int
	MaxBuffered   int
}

/*
DefaultStreamingConfig returns the default streaming configuration.

Defaults are chosen to balance throughput and memory usage for typical
source-file workloads while remaining safe for moderate-sized streams.

Current defaults:

  - ReadChunkSize: 25
  - MaxBuffered:   30

These values may be tuned per workload via the configuration builder.
*/
func DefaultStreamingConfig() StreamingConfig {
	return StreamingConfig{
		ReadChunkSize: 25,
		MaxBuffered:   30,
	}
}

// ============================================================
// LANG PARSER CONFIGURATION
// ============================================================

/*
LangParserConfiguration defines the runtime and resource configuration
for a language parser instance.

This structure is intentionally limited to operational concerns only.

It does NOT contain language semantics — those belong exclusively to LangSpec.

Concerns managed here include:

  - memory allocation strategy
  - automaton memory bounds
  - streaming behavior
  - performance tuning parameters

Separation of concerns:

  - LangSpec  → language semantics
  - Configuration → execution behavior
*/
type LangParserConfiguration[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
] struct {
	spec *LangSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

	scratchAllocationFn memarch.AllocationFn

	maxLexerAutomatonMemory memcore.MemoryUnitBytes

	streaming StreamingConfig
}

/*
LangParserConfigurationCreate constructs a runtime configuration
with safe and performant defaults.

Defaults:

  - Lexer automaton memory limit: 1 GB
  - Streaming configuration: DefaultStreamingConfig()

The caller is expected to tune memory and streaming parameters
for their workload when necessary.
*/
func LangParserConfigurationCreate[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
](
	spec *LangSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	scratch memarch.AllocationFn,
) *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {

	return &LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		spec:                    spec,
		scratchAllocationFn:     scratch,
		maxLexerAutomatonMemory: 1 * memcore.GigaByte,
		streaming:               DefaultStreamingConfig(),
	}
}

// ============================================================
// BUILDER API
// ============================================================

/*
WithMaxLexerAutomatonMemory configures the maximum memory allowed for
lexer automaton structures.

This limit protects against:

  - exponential state explosion
  - malformed grammars
  - hostile input patterns

If the limit is exceeded during automaton construction, lexer creation
should fail with a resource error.

Typical values:

  - small DSLs: tens of MB
  - programming languages: hundreds of MB to multiple GB
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithMaxLexerAutomatonMemory(
	amount memcore.MemoryUnitBytes,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	c.maxLexerAutomatonMemory = amount
	return c
}

/*
WithStreamingConfig replaces the entire streaming configuration block.

This is the preferred method when applying predefined profiles
(e.g. low-memory, high-throughput, debugging, benchmarking).

Example:

	cfg.WithStreamingConfig(StreamingConfig{
		ReadChunkSize: 128,
		MaxBuffered:   256,
	})
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithStreamingConfig(
	cfg StreamingConfig,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	c.streaming = cfg
	return c
}

/*
WithStreamingReadChunkSize sets the number of observations pulled from
the producer per streaming read.

Effects:

  - Larger size → higher throughput, higher latency, higher memory pressure
  - Smaller size → lower memory usage, more frequent calls

This may be tuned independently without affecting buffer limits.
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithStreamingReadChunkSize(
	size int,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	c.streaming.ReadChunkSize = size
	return c
}

/*
WithStreamingMaxBuffered sets the maximum number of observations buffered
by the streaming lexer session.

This enforces a hard memory ceiling for streaming input and provides
backpressure when the producer outpaces consumption.

Lower values reduce memory footprint.
Higher values improve throughput under bursty input.
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithStreamingMaxBuffered(
	max int,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	c.streaming.MaxBuffered = max
	return c
}

// ---------------------------------------------------------------- LANG PARSER

/* LangParser encapsulates the pipeline that transforms a source into an AST. */
type LangParser[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable] struct {
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

	lexer  *lexarch.Lexer[TObservation, TLexerState, TToken, TTokenRole]
	parser *syntaxa.SyntaxaParser[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]

	used      bool
	destroyed bool
}

/* LangParserCreate constructs a language parser instance. */
func LangParserCreate[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
) *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind] {
	lexer := lexarch.LexerCreate(
		config.spec.Lexer.rulesets,
		config.spec.Lexer.errorToken,
		config.spec.Lexer.eofToken,
		config.scratchAllocationFn,
		config.maxLexerAutomatonMemory,
	)

	parser := syntaxa.SyntaxaParserCreate(
		config.spec.Parser.ruleSelector,
		config.spec.Parser.rootNodeKind,
		config.spec.Parser.errorNodeKind,
		config.spec.Parser.freezeAfterParse,
	)

	return &LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind]{
		config:    config,
		lexer:     lexer,
		parser:    parser,
		used:      false,
		destroyed: false,
	}
}

/*
LangParserParseFile parses a source file into the AST as generated by the parsing engine.

If streaming is set to false, it will read the entire file into memory and pass that to the lexer.
If streaming is set to true, it will pass it to the lexer in chunks.

During streaming, the lexer will receive the configured max buffer and readChunkSize.

NOTE: mapFn can be zero if the TObservation is byte or rune. If TObservation is byte/rune, mapFn will not be used.
*/
func LangParserParseFile[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	streaming bool,
) (
	*syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind],
	*syntaxa.SyntaxErrors,
	error,
) {
	if langParser.destroyed {
		return nil, nil, fmt.Errorf("LangParser already destroyed")
	}

	if langParser.used {
		return nil, nil, fmt.Errorf(
			"LangParser is single-use; create a new instance per parse",
		)
	}

	langParser.used = true

	defer func() {
		if !langParser.destroyed {
			lexarch.LexerClose(langParser.lexer)
			langParser.destroyed = true
		}
	}()

	if !system.FileExists(sourceFile) {
		return nil, nil, fmt.Errorf("source file non-existent: %s", sourceFile)
	}

	syntaxErrors := &syntaxa.SyntaxErrors{
		Errors: make([]syntaxa.SyntaxError, 0),
	}

	parsingContext, err := getParsingContext(langParser, sourceFile, mapFn, syntaxErrors, streaming)
	if err != nil {
		return nil, nil, err
	}

	rootNode := parsingContext.Editor.NewNode(
		langParser.config.spec.Parser.rootNodeKind,
	)

	syntaxa.SyntaxaParserParseWithContext(
		langParser.parser,
		parsingContext,
		rootNode,
		langParser.config.spec.Lexer.eofToken,
	)

	return rootNode, syntaxErrors, nil
}

// ---------------------------------------------------------------- PRIVATE HELPERS

func getParsingContext[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	syntaxErrors *syntaxa.SyntaxErrors,
	streaming bool,
) (syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind], error) {
	var parsingContext syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

	if !streaming {
		ctx, err := buildSequentialParsingContext(
			langParser,
			sourceFile,
			mapFn,
			syntaxErrors,
		)
		if err != nil {
			return parsingContext, err
		}
		parsingContext = ctx
	} else {
		ctx, err := buildStreamingParsingContext(
			langParser,
			sourceFile,
			mapFn,
			syntaxErrors,
		)
		if err != nil {
			return parsingContext, err
		}
		parsingContext = ctx
	}

	return parsingContext, nil
}

func buildSequentialParsingContext[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	syntaxErrors *syntaxa.SyntaxErrors,
) (
	syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	error,
) {
	sourceInput, err := getSourceInput(sourceFile, mapFn)
	if err != nil {
		var zero syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]
		return zero, fmt.Errorf("could not decode source-file: %w", err)
	}

	lexingSession := lexarch.LexerSessionCreate(
		langParser.config.spec.Lexer.initialState,
		sourceInput,
		langParser.config.spec.Lexer.newlineDetect,
	)

	parsingContext := syntaxa.BuildExecRuleContextFromLexerSession(
		langParser.parser,
		langParser.lexer,
		lexingSession,
		syntaxErrors,
	)

	return parsingContext, nil
}

func buildStreamingParsingContext[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	syntaxErrors *syntaxa.SyntaxErrors,
) (syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind], error) {
	producer, err := newFileObservationProducer(
		sourceFile,
		mapFn,
		32*1024,
	)

	if err != nil {
		var zero syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]
		return zero, fmt.Errorf("FS: open failed: %w", err)
	}

	lexingSession := lexarch.StreamingLexerSessionCreate(
		langParser.config.spec.Lexer.initialState,
		producer,
		langParser.config.spec.Lexer.newlineDetect,
		langParser.config.streaming.ReadChunkSize,
		langParser.config.streaming.MaxBuffered,
	)

	parsingContext := syntaxa.BuildExecRuleContextFromStreamingSession(
		langParser.parser,
		langParser.lexer,
		lexingSession,
		syntaxErrors,
	)

	return parsingContext, nil
}

func newFileObservationProducer[TObservation cmp.Ordered](
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	bufSize int,
) (lexarch.ObservationProducerFn[TObservation], error) {
	const channelCapacity = 8

	chunks := make(chan []TObservation, channelCapacity)
	errCh := make(chan error, 1)

	var startStream func() error

	switch any(*new(TObservation)).(type) {

	case rune:
		startStream = func() error {
			return system.FileStreamRunes(sourceFile, bufSize, func(rs []rune) error {
				out := make([]TObservation, len(rs))
				for i, r := range rs {
					out[i] = TObservation(r)
				}
				chunks <- out
				return nil
			})
		}

	case byte:
		startStream = func() error {
			return system.FileStreamBytes(sourceFile, bufSize, func(bs []byte) error {
				out := make([]TObservation, len(bs))
				for i := range bs {
					out[i] = TObservation(bs[i])
				}
				chunks <- out
				return nil
			})
		}

	default:
		if mapFn == nil {
			return nil, fmt.Errorf(
				"streaming producer: mapFn required for non-byte and non-rune observations",
			)
		}

		startStream = func() error {
			return system.FileStreamAs(
				sourceFile,
				bufSize,
				mapFn,
				func(chunk []TObservation) error {
					out := make([]TObservation, len(chunk))
					copy(out, chunk)
					chunks <- out
					return nil
				},
			)
		}
	}

	started := false

	var pending []TObservation

	return func(dst []TObservation) (n int, done bool, err error) {

		if !started {
			started = true

			go func() {
				errCh <- startStream()
				close(chunks)
			}()
		}

		if len(pending) > 0 {
			n = min(len(dst), len(pending))
			copy(dst, pending[:n])
			pending = pending[n:]
			return n, false, nil
		}

		next, ok := <-chunks
		if ok {
			pending = next

			n = min(len(dst), len(pending))
			copy(dst, pending[:n])
			pending = pending[n:]
			return n, false, nil
		}

		streamErr := <-errCh
		if streamErr != nil {
			return 0, false, streamErr
		}

		return 0, true, nil
	}, nil
}

func getSourceInput[TObservation cmp.Ordered](
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
) ([]TObservation, error) {

	var zero TObservation

	switch any(zero).(type) {

	case rune:
		runes, err := system.FileReadAllRunes(sourceFile)
		if err != nil {
			return nil, err
		}

		out := make([]TObservation, len(runes))
		for i, r := range runes {
			out[i] = TObservation(r)
		}
		return out, nil

	case byte:
		bytes, err := system.FileReadAllBytes(sourceFile)
		if err != nil {
			return nil, err
		}

		out := make([]TObservation, len(bytes))
		for i, b := range bytes {
			out[i] = TObservation(b)
		}
		return out, nil

	default:
		if mapFn == nil {
			return nil, fmt.Errorf(
				"getSourceInput: mapFn required for non-byte and non-rune observation type",
			)
		}
		return system.FileReadAllAs(sourceFile, mapFn)
	}
}
