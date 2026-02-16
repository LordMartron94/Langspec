package langspec

import (
	"cmp"
	"fmt"
	"foundation/extensions"
	"foundation/system"
	"lexarch"
	"memarch"
	"memcore"
	"sync/atomic"
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

type ValidationSeverity uint8

const (
	VALIDATION_SEVERITY_DIAGNOSTIC ValidationSeverity = iota + 1
	VALIDATION_SEVERITY_INFO
	VALIDATION_SEVERITY_WARNING
	VALIDATION_SEVERITY_ERROR
	VALIDATION_SEVERITY_FATAL
)

/* ParserSyntaxErrorHook is an optional hook that runs post-parsing to process syntax errors. */
type ParserSyntaxErrorHook func(errors *syntaxa.SyntaxErrors) error

/* ValidationEntry is a single validation error. */
type ValidationEntry[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	// Stable identifier (for tooling, tests, suppression, docs)
	Code string

	// Human message
	Message string

	// How serious this is
	Severity ValidationSeverity

	// Where it happened (prefer AST node — spans can be derived)
	Node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]

	// Optional explicit span override (for tokens, ranges, etc.)
	Start int
	End   int

	// Optional extra context
	Notes []string
}

/* StageValidationResult encapsulates a result of stage validation. */
type StageValidationResult[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	StageName string
	Order     int
	Entries   []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]
}

/* ValidationEntries encapsulates the errors during validation. */
type ValidationEntries[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	Results []StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind]
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) getStageResult(stageName string) *StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind] {
	for i := range v.Results {
		if v.Results[i].StageName == stageName {
			return &v.Results[i]
		}
	}

	return nil
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) beginStage(stage *ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]) {
	v.Results = append(v.Results, StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind]{
		StageName: stage.Name,
		Order:     stage.Order,
		Entries:   []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]{},
	})
}

/*
GetStageEntries returns the entries for a given stage.

If the stage does not exist or doesn't have a result struct associated, it will return nil.
*/
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) GetStageEntries(stageName string) []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind] {
	if result := v.getStageResult(stageName); result != nil {
		return result.Entries
	}

	return nil
}

func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) reportForStage(stage *ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind], entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]) {
	result := v.getStageResult(stage.Name)
	result.Entries = append(result.Entries, entry)
}

/* CountBySeverity shows the amount of entries having this severity. */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) CountBySeverity(severity ValidationSeverity) int {
	amount := 0

	for _, result := range v.Results {
		for _, entry := range result.Entries {
			if entry.Severity == severity {
				amount++
			}
		}
	}

	return amount
}

/* HasErrors checks if there are errors (including fatals). */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) HasErrors() bool {
	for _, result := range v.Results {
		for _, entry := range result.Entries {
			if entry.Severity >= VALIDATION_SEVERITY_ERROR {
				return true
			}
		}
	}

	return false
}

/* HasFatal checks if there are fatal errors. */
func (v *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]) HasFatal() bool {
	for _, result := range v.Results {
		for _, entry := range result.Entries {
			if entry.Severity == VALIDATION_SEVERITY_FATAL {
				return true
			}
		}
	}

	return false
}

/* ASTValidationStageContext encapsulates the available functions for a validation stage. */
type ASTValidationStageContext[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	RootNode *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]

	NewValidationEntry func(code, message string, severity ValidationSeverity, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]

	ReportDiagnostic func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportInfo       func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportWarning    func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportError      func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind])
	ReportFatal      func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind])

	ReportValidationEntry func(entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind])
}

/*
ASTValidationStageProcessor processes a single validation stage.

Errors encountered during validation should be reported using the context.
*/
type ASTValidationStageProcessor[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] func(
	ctx *ASTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind],
)

/*
ASTValidationStageSummarizer summarizes the validation errors from this stage.

This is commonly used for logging and user feedback.
*/
type ASTValidationStageSummarizer[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] func(
	stage *ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind],
	entries []ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind],
)

/*
ASTValidationStage encapsulates a single validation stage.

Stages with a higher order are executed later (stage 0 executes before stage 100)
*/
type ASTValidationStage[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable] struct {
	Name        string
	Description string

	Order int

	Processor ASTValidationStageProcessor[TObservation, TToken, TTokenRole, TNodeKind]
}

/* ASTValidationStageCreate creates an ASTValidationStage instance. */
func ASTValidationStageCreate[TObservation cmp.Ordered, TToken, TTokenRole, TNodeKind comparable](
	name, description string,
	order int,
	processor ASTValidationStageProcessor[TObservation, TToken, TTokenRole, TNodeKind],
) *ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind] {
	return &ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]{
		Name:        name,
		Description: description,
		Processor:   processor,
	}
}

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

	errorHook ParserSyntaxErrorHook

	validationStages []*ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]

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
		errorHook:        nil,
		validationStages: make([]*ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind], 0),
	}
}

/* WithErrorHook configures the post-processor for the error hook. */
func (p *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithErrorHook(
	hook ParserSyntaxErrorHook,
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	p.errorHook = hook
	return p
}

/* WithValidationStages adds validation stages to the parser spec. */
func (p *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithValidationStages(
	stages ...*ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind],
) *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	p.validationStages = append(p.validationStages, stages...)
	return p
}

func (p *ParserSpec[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) getSortedStages() []*ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind] {
	sorted := extensions.SortedCopyShallow(p.validationStages, func(a, b *ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind]) int {
		return cmp.Compare(a.Order, b.Order)
	})

	return sorted
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
	stageReporter   ASTValidationStageSummarizer[TObservation, TToken, TTokenRole, TNodeKind]
}

/*
LangParserConfigurationCreate constructs a runtime configuration
with safe and performant defaults.

Defaults:

  - Lexer automaton memory limit: 1 GB
  - Streaming configuration: DefaultStreamingConfig()

The caller is expected to tune memory and streaming parameters
for their workload when necessary.

The stageReporter can be nil if the client does not want automatic stage validation reporting during the validation phase.
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
	stageReporter ASTValidationStageSummarizer[TObservation, TToken, TTokenRole, TNodeKind],
) *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	return &LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]{
		spec:                    spec,
		scratchAllocationFn:     scratch,
		maxLexerAutomatonMemory: 1 * memcore.GigaByte,
		streaming:               DefaultStreamingConfig(),
		stageReporter:           stageReporter,
		forceValidation:         false,
	}
}

// ============================================================
// BUILDER API
// ============================================================

/* WithForceValidation configures whether to force AST validation. */
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

/* LangParser encapsulates the pipeline that transforms a source into an AST. */
type LangParser[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable] struct {
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

	lexer  *lexarch.Lexer[TObservation, TLexerState, TToken, TTokenRole]
	parser *syntaxa.SyntaxaParser[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]

	lexingSessionCache          *lexarch.LexerSession[TObservation, TLexerState]
	lexingStreamingSessionCache *lexarch.StreamingLexerSession[TObservation, TLexerState]

	destroyed atomic.Bool
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
		config: config,
		lexer:  lexer,
		parser: parser,
	}
}

/*
LangParserParseFile parses a source file into the AST as generated by the parsing engine.
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
	*syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind],
	*syntaxa.SyntaxErrors,
	*ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind],
	error,
) {
	ensureAlive(langParser)

	session.begin()
	defer session.end()

	if !system.FileExists(session.sourceFile) {
		return nil, nil, nil, fmt.Errorf("source file non-existent: %s", session.sourceFile)
	}

	syntaxErrors := &syntaxa.SyntaxErrors{
		Errors: make([]syntaxa.SyntaxError, 0),
	}

	parsingContext, err := getParsingContext(langParser, session.sourceFile, session.mapFn, syntaxErrors, session.streaming)
	if err != nil {
		return nil, nil, nil, err
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

	errorHook := langParser.config.spec.Parser.errorHook
	if errorHook != nil {
		if err := errorHook(syntaxErrors); err != nil {
			return rootNode, syntaxErrors, nil, err
		}
	}

	validationEntries := &ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind]{
		Results: make([]StageValidationResult[TObservation, TToken, TTokenRole, TNodeKind], 0),
	}

	if !syntaxErrors.HasErrors() || langParser.config.forceValidation {
		validationStages := langParser.config.spec.Parser.getSortedStages()
		for _, stage := range validationStages {
			validationEntries.beginStage(stage)

			validationCtx := buildValidationContextForStage(stage, rootNode, validationEntries)
			stage.Processor(validationCtx)

			if langParser.config.stageReporter != nil {
				result := validationEntries.GetStageEntries(stage.Name)
				langParser.config.stageReporter(stage, result)
			}

			if validationEntries.HasFatal() {
				return rootNode, syntaxErrors, validationEntries,
					fmt.Errorf("stopped because of a fatal validation stage")
			}
		}

	}

	return rootNode, syntaxErrors, validationEntries, nil
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

func buildValidationContextForStage[
	TObservation cmp.Ordered,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	stage *ASTValidationStage[TObservation, TToken, TTokenRole, TNodeKind],
	rootNode *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind],
	sharedValidationEntries *ValidationEntries[TObservation, TToken, TTokenRole, TNodeKind],
) *ASTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind] {
	newValidationEntry := func(code, message string, severity ValidationSeverity, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind] {
		return ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]{
			Code:     code,
			Message:  message,
			Severity: severity,
			Node:     node,
		}
	}

	reportValidationEntry := func(entry ValidationEntry[TObservation, TToken, TTokenRole, TNodeKind]) {
		sharedValidationEntries.reportForStage(stage, entry)
	}

	return &ASTValidationStageContext[TObservation, TToken, TTokenRole, TNodeKind]{
		RootNode:           rootNode,
		NewValidationEntry: newValidationEntry,
		ReportDiagnostic: func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_DIAGNOSTIC, node)
			reportValidationEntry(entry)
		},
		ReportInfo: func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_INFO, node)
			reportValidationEntry(entry)
		},
		ReportWarning: func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_WARNING, node)
			reportValidationEntry(entry)
		},
		ReportError: func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_ERROR, node)
			reportValidationEntry(entry)
		},
		ReportFatal: func(code, message string, node *syntaxa.SyntaxaASTNode[TObservation, TToken, TTokenRole, TNodeKind]) {
			entry := newValidationEntry(code, message, VALIDATION_SEVERITY_FATAL, node)
			reportValidationEntry(entry)
		},
		ReportValidationEntry: reportValidationEntry,
	}
}

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
) *lexarch.LexerSession[TObservation, TLexerState] {
	if langParser.lexingSessionCache == nil {
		session := lexarch.LexerSessionCreate(
			langParser.config.spec.Lexer.initialState,
			sourceInput,
			langParser.config.spec.Lexer.newlineDetect,
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
) *lexarch.StreamingLexerSession[TObservation, TLexerState] {
	if langParser.lexingStreamingSessionCache == nil {
		session := lexarch.StreamingLexerSessionCreate(
			langParser.config.spec.Lexer.initialState,
			producer,
			langParser.config.spec.Lexer.newlineDetect,
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
