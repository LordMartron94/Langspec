package langspec

import (
	"autarch"
	"autarch/pattern"
	"cmp"
	"fmt"
	"foundation/domain"
	"foundation/system"
	"lexarch"
	"memarch"
	"memcore"
	"strings"
	"sync/atomic"
	"syntaxa"
)

// ------------------------------------------------------------ LEXER SPEC

/* LexerSpec defines the complete lexing protocol for a language. */
type LexerSpec[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState comparable] struct {
	rulesets map[TLexerState]lexarch.LexingRuleset[TObservation, TToken, TTokenRole]

	initialState TLexerState

	newlineDetect   lexarch.NewlineDetector[TObservation]
	columnAdvanceFn lexarch.ColumnAdvanceFn[TObservation]
	toBytes         func(observations []TObservation) []byte

	observationFormatter lexarch.ObservationFormatter[TObservation]
	observationDomain    *domain.DiscreteDomain[TObservation]

	tokenFormatter func(token TToken) string
	dfaFormatter   *autarch.DFADebugFormatter[TObservation, pattern.AnnotatedOutcome[lexarch.TokenOutcome[TToken, TTokenRole]]]

	compilerMode lexarch.CompilerMode

	eofToken TToken
}

/* LexerSpecCreate constructs a lexer specification. */
func LexerSpecCreate[TObservation cmp.Ordered, TToken, TTokenRole, TLexerState comparable](
	eofToken TToken,
	initialState TLexerState,
	newlineDetect lexarch.NewlineDetector[TObservation],
	columnAdvanceFn lexarch.ColumnAdvanceFn[TObservation],
	toBytes func(observations []TObservation) []byte,
	observationFormatter lexarch.ObservationFormatter[TObservation],
	observationDomain *domain.DiscreteDomain[TObservation],
	tokenFormatter func(token TToken) string,
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	return &LexerSpec[TObservation, TToken, TTokenRole, TLexerState]{
		rulesets:             make(map[TLexerState]lexarch.LexingRuleset[TObservation, TToken, TTokenRole]),
		initialState:         initialState,
		newlineDetect:        newlineDetect,
		columnAdvanceFn:      columnAdvanceFn,
		toBytes:              toBytes,
		eofToken:             eofToken,
		observationFormatter: observationFormatter,
		observationDomain:    observationDomain,
		tokenFormatter:       tokenFormatter,
		compilerMode:         lexarch.Glushkov,
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

func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) WithDFADebugFormatter(
	f *autarch.DFADebugFormatter[TObservation, pattern.AnnotatedOutcome[lexarch.TokenOutcome[TToken, TTokenRole]]],
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	l.dfaFormatter = f
	return l
}

func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) WithCompilationMode(
	mode lexarch.CompilerMode,
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	l.compilerMode = mode
	return l
}

// ------------------------------------------------------------ PARSER SPEC

/* ParserSyntaxErrorHook is an optional hook that runs post-parsing to process syntax errors. */
type ParserSyntaxErrorHook[TObservation cmp.Ordered] func(errors *syntaxa.SyntaxErrors[TObservation]) error

/* ParserSpec defines the grammar and LST contract. */
type ParserSpec[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
] struct {
	rootNodeKind  TNodeKind
	errorNodeKind TNodeKind

	grammarPackage    *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]
	registry          syntaxa.RuleRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]
	nodePostProcessor syntaxa.NodePostProcessor[TObservation, TToken, TTokenRole, TNodeKind]

	errorHook ParserSyntaxErrorHook[TObservation]

	defaultSkipRoles []TTokenRole

	freezeAfterParse bool
}

/*
	ParserSpecCreate constructs a parser specification from a grammar package and rule registry.

The package must have been produced with an entry rule (ProducePackage(..., &programRule)).
The registry maps grammar labels to parser rules for context-boundary productions; typically
obtained from the same RuleBuilder used to build the grammar (RuleBuilderGetRegistry).
*/
func ParserSpecCreate[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind comparable,
](
	grammarPackage *syntaxa.GrammarPackage[TObservation, TToken, TTokenRole, TNodeKind, TLexerState],
	registry syntaxa.RuleRegistry[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	rootNodeKind, errorNodeKind TNodeKind,
	freezeAfterParse bool,
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	return &ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		grammarPackage:    grammarPackage,
		registry:          registry,
		rootNodeKind:      rootNodeKind,
		errorNodeKind:     errorNodeKind,
		freezeAfterParse:  freezeAfterParse,
		nodePostProcessor: nil,
		errorHook:         nil,
		defaultSkipRoles:  make([]TTokenRole, 0),
	}
}

/* WithErrorHook configures the post-processor for the error hook. */
func (p *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithErrorHook(
	hook ParserSyntaxErrorHook[TObservation],
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	p.errorHook = hook
	return p
}

/* WithSkipRoles adds skip roles to the parser spec. */
func (p *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithSkipRoles(
	roles ...TTokenRole,
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	p.defaultSkipRoles = append(p.defaultSkipRoles, roles...)
	return p
}

func (p *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithPostProcessor(
	postProcessor syntaxa.NodePostProcessor[TObservation, TToken, TTokenRole, TNodeKind],
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	p.nodePostProcessor = postProcessor
	return p
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

	forceValidation bool
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
		forceValidation:         false,
	}
}

// ============================================================
// BUILDER API
// ============================================================

/* WithForceValidation configures whether to force LST validation. */
func (c *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithForceValidation(v bool) *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	c.forceValidation = v
	return c
}

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

// ---------------------------------------------------------------- SESSION

/*
LangParserSession encapsulates a single parsing session.

While the LangParser is safe to re-use and use concurrently, the session should only be used once.

The session can be reset by calling Reset, which allows it to be used once more.
*/
type LangParserSession[TObservation cmp.Ordered] struct {
	sourceFile string
	mapFn      func([]byte) ([]TObservation, error)
	streaming  bool

	inUse atomic.Bool
}

/*
LangParserSessionCreate creates a lang parser session instance.

If streaming is set to false, it will read the entire file into memory and pass that to the lexer.
If streaming is set to true, it will pass it to the lexer in chunks.

During streaming, the lexer will receive the configured max buffer and readChunkSize.

NOTE: mapFn can be zero if the TObservation is byte or rune. If TObservation is byte/rune, mapFn will not be used.
*/
func LangParserSessionCreate[TObservation cmp.Ordered](
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	streaming bool,
) *LangParserSession[TObservation] {
	session := &LangParserSession[TObservation]{
		sourceFile: sourceFile,
		mapFn:      mapFn,
		streaming:  streaming,
	}
	session.inUse.Store(false)
	return session
}

func (l *LangParserSession[TObservation]) begin() {
	if l.inUse.Load() {
		panic("LangParserSession is already in use (concurrent or re-entrant use detected)")
	}

	l.inUse.Store(true)
}

func (l *LangParserSession[TObservation]) end() {
	l.inUse.Store(false)
}

/*
Reset allows the lang parser session to be re-used.

If streaming is set to false, it will read the entire file into memory and pass that to the lexer.
If streaming is set to true, it will pass it to the lexer in chunks.

During streaming, the lexer will receive the configured max buffer and readChunkSize.

NOTE: mapFn can be zero if the TObservation is byte or rune. If TObservation is byte/rune, mapFn will not be used.
*/
func (l *LangParserSession[TObservation]) Reset(
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	streaming bool,
) {
	if l.inUse.Load() {
		panic("Cannot reset an active lang-parser session.")
	}

	l.sourceFile = sourceFile
	l.mapFn = mapFn
	l.streaming = streaming

	l.inUse.Store(false)
}

// ---------------------------------------------------------------- LANG PARSER

/* LangParser encapsulates the pipeline that transforms a source into an LST. */
type LangParser[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable] struct {
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

	lexer  *lexarch.Lexer[TObservation, TLexerState, TToken, TTokenRole]
	parser *syntaxa.SyntaxaParser[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]

	lexingSessionCache          *lexarch.LexerSession[TObservation, TLexerState, TToken]
	lexingStreamingSessionCache *lexarch.StreamingLexerSession[TObservation, TLexerState, TToken]

	destroyed atomic.Bool
}

/*
LangParserLexerCreateFromSpec creates a lexer according to the spec.

This is useful if clients want to delegate lexing creation without relying on the rest of LSpec's LangParser.
*/
func LangParserLexerCreateFromSpec[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole comparable](
	scratchAllocationFn memarch.AllocationFn,
	maxLexerAutomatonMemory memcore.MemoryUnitBytes,
	spec *LexerSpec[TObservation, TToken, TTokenRole, TLexerState],
) *lexarch.Lexer[TObservation, TLexerState, TToken, TTokenRole] {
	return lexarch.LexerCreate(
		spec.rulesets,
		spec.eofToken,
		scratchAllocationFn,
		maxLexerAutomatonMemory,
		lexarch.ObservationCTXCreate(
			spec.observationFormatter,
			spec.observationDomain,
			spec.toBytes,
		),
		spec.compilerMode,
	)
}

/* LangParserCreate constructs a language parser instance. */
func LangParserCreate[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
) *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind] {
	lexer := LangParserLexerCreateFromSpec(config.scratchAllocationFn, config.maxLexerAutomatonMemory, config.spec.Lexer)

	parser := syntaxa.SyntaxaParserCreate(
		config.spec.Parser.grammarPackage,
		config.spec.Parser.registry,
		config.spec.Lexer.tokenFormatter,
		config.spec.Lexer.observationFormatter,
		config.spec.Parser.nodePostProcessor,
		config.spec.Lexer.eofToken,
		config.spec.Parser.rootNodeKind,
		config.spec.Parser.errorNodeKind,
		config.spec.Parser.freezeAfterParse,
	)
	parser.SetDefaultSkips(config.spec.Parser.defaultSkipRoles...)
	parser.EnableTrace(true)

	return &LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind]{
		config: config,
		lexer:  lexer,
		parser: parser,
	}
}

/* GetGrammarPackage returns the grammar package used by the parser (for debug dumps, editor IR, etc.). */
func (p *LangParser[TObs, TLexerState, TToken, TTokenRole, TNodeKind]) GetGrammarPackage() *syntaxa.GrammarPackage[TObs, TToken, TTokenRole, TNodeKind, TLexerState] {
	return p.config.spec.Parser.grammarPackage
}

func (p *LangParser[TObs, TLexerState, TToken, TTokenRole, TNodeKind]) DebugDumpLexerDFA(
	state TLexerState,
) string {

	spec := p.config.spec.Lexer

	if spec.dfaFormatter == nil {
		return "DFA debug formatter not configured"
	}

	return lexarch.LexerDebugDFA(
		p.lexer,
		state,
		spec.dfaFormatter,
	)
}

func (p *LangParser[TObs, TLexerState, TToken, TTokenRole, TNodeKind]) DebugDumpAllLexerDFAs() string {

	spec := p.config.spec.Lexer
	if spec.dfaFormatter == nil {
		return "DFA debug formatter not configured"
	}

	var out strings.Builder

	for state := range spec.rulesets {
		out.WriteString("=== DFA for state ")
		out.WriteString(fmt.Sprint(state))
		out.WriteString(" ===\n")
		out.WriteString(
			lexarch.LexerDebugDFA(p.lexer, state, spec.dfaFormatter),
		)
		out.WriteString("\n")
	}

	return out.String()
}

/*
LangParserLexFile only lexes the file and returns the stream of lexemes until EOF.

This can be useful for custom pipelines or debugging.
*/
func LangParserLexFile[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	session *LangParserSession[TObservation],
) ([]lexarch.Lexeme[TObservation, TToken, TTokenRole], error) {
	sourceInput, err := getSourceInput(session.sourceFile, session.mapFn)
	if err != nil {
		return nil, fmt.Errorf("could not decode source-file: %w", err)
	}

	lexingSession := getLexerSession(langParser, sourceInput)

	out := make([]lexarch.Lexeme[TObservation, TToken, TTokenRole], 0)

	for {
		current := lexarch.LexerConsume(langParser.lexer, lexingSession)

		out = append(out, current)
		if current.Token == langParser.config.spec.Lexer.eofToken {
			break
		}
	}

	return out, nil
}

/*
LangParserParseFile parses a source file into the LST as generated by the parsing engine.
*/
func LangParserParseFile[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	session *LangParserSession[TObservation],
) (
	*syntaxa.ParseTrace[TToken],
	*syntaxa.SyntaxaLSTNode[TObservation, TToken, TTokenRole, TNodeKind],
	*syntaxa.SyntaxErrors[TObservation],
	error,
) {
	ensureAlive(langParser)

	session.begin()
	defer session.end()

	if !system.FileExists(session.sourceFile) {
		return nil, nil, nil, fmt.Errorf("source file non-existent: %s", session.sourceFile)
	}

	syntaxErrors := &syntaxa.SyntaxErrors[TObservation]{
		Errors: make([]syntaxa.SyntaxError[TObservation], 0),
	}

	parsingContext, err := getParsingContext(langParser, session.sourceFile, session.mapFn, syntaxErrors, session.streaming)
	if err != nil {
		return nil, nil, nil, err
	}

	rootNode, trace, err := syntaxa.SyntaxaParserParseWithContext(
		langParser.parser,
		parsingContext,
	)

	if err != nil {
		return trace, rootNode, syntaxErrors, err
	}

	errorHook := langParser.config.spec.Parser.errorHook
	if errorHook != nil {
		if err := errorHook(syntaxErrors); err != nil {
			return trace, rootNode, syntaxErrors, err
		}
	}

	return trace, rootNode, syntaxErrors, nil
}

/*
LangParserDestroy cleans up manually allocated resources, invalidating further use of this LangParser instance.

Forgetting to call this results in memory leaks.
*/
func LangParserDestroy[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
) {
	lexarch.LexerClose(langParser.lexer)
	langParser.destroyed.Store(true)
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
	syntaxErrors *syntaxa.SyntaxErrors[TObservation],
	streaming bool,
) (*syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind], error) {
	var parsingContext *syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

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
	syntaxErrors *syntaxa.SyntaxErrors[TObservation],
) (
	*syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	error,
) {
	sourceInput, err := getSourceInput(sourceFile, mapFn)
	if err != nil {
		return nil, fmt.Errorf("could not decode source-file: %w", err)
	}

	lexingSession := getLexerSession(langParser, sourceInput)

	parsingContext := syntaxa.BuildExecRuleContextFromLexerSession(
		langParser.parser,
		langParser.lexer,
		lexingSession,
		syntaxErrors,
	)

	return parsingContext, nil
}

func getLexerSession[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceInput []TObservation,
) *lexarch.LexerSession[TObservation, TLexerState, TToken] {
	if langParser.lexingSessionCache == nil {
		session := lexarch.LexerSessionCreate[TObservation, TLexerState, TToken](
			langParser.config.spec.Lexer.initialState,
			sourceInput,
			langParser.config.spec.Lexer.newlineDetect,
			langParser.config.spec.Lexer.columnAdvanceFn,
		)
		langParser.lexingSessionCache = session
		return session
	}

	langParser.lexingSessionCache.Reset(sourceInput, langParser.config.spec.Lexer.initialState)
	return langParser.lexingSessionCache
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
	syntaxErrors *syntaxa.SyntaxErrors[TObservation],
) (*syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind], error) {
	producer, err := newFileObservationProducer(
		sourceFile,
		mapFn,
		32*1024,
	)

	if err != nil {
		return nil, fmt.Errorf("FS: open failed: %w", err)
	}

	lexingSession := getLexerStreamingSession(langParser, producer)

	parsingContext := syntaxa.BuildExecRuleContextFromStreamingSession(
		langParser.parser,
		langParser.lexer,
		lexingSession,
		syntaxErrors,
	)

	return parsingContext, nil
}

func getLexerStreamingSession[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	producer lexarch.ObservationProducerFn[TObservation],
) *lexarch.StreamingLexerSession[TObservation, TLexerState, TToken] {
	if langParser.lexingStreamingSessionCache == nil {
		session := lexarch.StreamingLexerSessionCreate[TObservation, TLexerState, TToken](
			langParser.config.spec.Lexer.initialState,
			producer,
			langParser.config.spec.Lexer.newlineDetect,
			langParser.config.spec.Lexer.columnAdvanceFn,
			langParser.config.streaming.ReadChunkSize,
			langParser.config.streaming.MaxBuffered,
		)
		langParser.lexingStreamingSessionCache = session
		return session
	}

	langParser.lexingStreamingSessionCache.Reset(producer, langParser.config.spec.Lexer.initialState)
	return langParser.lexingStreamingSessionCache
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

func ensureAlive[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
) {
	if langParser.destroyed.Load() {
		panic("LangParser re-used after destroy")
	}
}
