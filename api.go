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
	"time"
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
	scanConfig   lexarch.LexerScanConfig
	positionMode lexerSpecPositionMode
	runeTabWidth int

	eofToken TToken
}

type lexerSpecPositionMode int

const (
	lexerSpecPositionModeGeneric lexerSpecPositionMode = iota + 1
	lexerSpecPositionModeRuneFast
	lexerSpecPositionModeByteFast
)

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
		scanConfig:           lexarch.LexerScanConfigDefault(),
		positionMode:         lexerSpecPositionModeGeneric,
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

/* Ruleset returns the ruleset for the given lexer state. Returns a zero ruleset if the state is not registered. */
func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) Ruleset(
	state TLexerState,
) lexarch.LexingRuleset[TObservation, TToken, TTokenRole] {
	return l.rulesets[state]
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

/* WithScanConfig replaces lexer scan configuration (stats, ForceRawCopy). */
func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) WithScanConfig(
	cfg lexarch.LexerScanConfig,
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	l.scanConfig = cfg
	return l
}

func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) WithRunePositionTrackingFast(
	tabWidth int,
) *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	if tabWidth <= 0 {
		panic("tabWidth must be > 0")
	}
	l.positionMode = lexerSpecPositionModeRuneFast
	l.runeTabWidth = tabWidth
	return l
}

func (l *LexerSpec[TObservation, TToken, TTokenRole, TLexerState]) WithBytePositionTrackingFast() *LexerSpec[TObservation, TToken, TTokenRole, TLexerState] {
	l.positionMode = lexerSpecPositionModeByteFast
	l.runeTabWidth = 0
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

	// getAnalysis supplies nullable/first/follow for the parser; optional (e.g. lowering.GetAnalysis(grammarPackage)).
	getAnalysis func() *syntaxa.GrammarAnalysis[TToken]
}

/*
	ParserSpecCreate constructs a parser specification from a grammar package and rule registry.

The package must have been produced with an entry rule (ProducePackage(..., &programRule)).
The registry maps grammar labels to parser rules for context-boundary productions; typically
obtained from the same RuleBuilder used to build the grammar (RuleBuilderGetRegistry).
getAnalysis is optional; pass syntaxa/lowering.GetAnalysis(grammarPackage) when the parser
needs nullable/first/follow (e.g. for Predict or Pratt), or nil.
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
	getAnalysis func() *syntaxa.GrammarAnalysis[TToken],
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
		getAnalysis:       getAnalysis,
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

	nfaToDFAPipelineMinTemp, nfaToDFAPipelineMaxTemp memcore.MemoryUnitBytes

	// nodePoolPrefill is an optional override for syntaxa.SyntaxaParser.SetNodePoolPrefill.
	// When zero, LangParser derives a hint from loaded source length.
	nodePoolPrefill int

	// nodePoolGrowFn is an optional override for syntaxa.SyntaxaParser.SetNodePoolGrowFn (nil = syntaxa default batching).
	nodePoolGrowFn func(currentCap, needed int) int

	forceValidation bool

	// collectParseTrace enables syntaxa parse trace event collection (high allocation cost).
	// When false, LangParserParseFile returns a nil trace on success.
	collectParseTrace bool
}

/*
LangParserConfigurationCreate constructs a runtime configuration
with safe and performant defaults.

Defaults:

  - Lexer automaton memory limit: 1 GB
  - NFA-to-DFA pipeline temp memory: min 1 KB, max 1 GB

The caller is expected to tune memory parameters for their workload when necessary.
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
		nfaToDFAPipelineMinTemp: 1 * memcore.KiloByte,
		nfaToDFAPipelineMaxTemp: 1 * memcore.GigaByte,
		nodePoolPrefill:         0,
		forceValidation:         false,
		collectParseTrace:       false,
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
WithCollectParseTrace enables recording a full syntaxa parse trace for each parse.

When false (default), parse traces are not collected and LangParserParseFile returns a nil trace on success, reducing allocations. Use LangParser.SetCollectParseTrace for per-parse toggling on a long-lived parser.
*/
func (c *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]) WithCollectParseTrace(v bool) *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind] {
	c.collectParseTrace = v
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
WithNFAToDFAPipelineTempMemory configures the temporary allocator bounds used during
NFA-to-DFA conversion and DFA minimization. Subset construction and Hopcroft minimization
use a temporary allocator; if its use exceeds maxTemp, conversion panics.

Typical values: min 1 KB–1 MB, max hundreds of MB to 1 GB depending on pattern complexity.
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithNFAToDFAPipelineTempMemory(
	minTemp, maxTemp memcore.MemoryUnitBytes,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	c.nfaToDFAPipelineMinTemp = minTemp
	c.nfaToDFAPipelineMaxTemp = maxTemp
	return c
}

/*
WithNodePoolPrefill sets a fixed Syntaxa LST node pool prefill count on the parser before each parse.

When hint is 0 (the default), LangParser computes a hint from the loaded source or file size.
When hint is greater than zero, that value is used instead of the heuristic.
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithNodePoolPrefill(
	hint int,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	if hint < 0 {
		panic("WithNodePoolPrefill: hint must be >= 0")
	}
	c.nodePoolPrefill = hint
	return c
}

/*
WithNodePoolGrowFn sets a custom Syntaxa LST node pool growth policy (see syntaxa.SyntaxaParser.SetNodePoolGrowFn).

Pass nil to use the default growth policy inside syntaxa.
*/
func (c *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
]) WithNodePoolGrowFn(
	growFn func(currentCap, needed int) int,
) *LangParserConfiguration[
	TObservation,
	TToken,
	TTokenRole,
	TLexerState,
	TNodeKind,
] {
	c.nodePoolGrowFn = growFn
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

	inUse atomic.Bool
}

/*
LangParserSessionCreate creates a lang parser session instance.

The full source file is read into memory and passed to the lexer session.

NOTE: mapFn can be zero if the TObservation is byte or rune. If TObservation is byte/rune, mapFn will not be used.
*/
func LangParserSessionCreate[TObservation cmp.Ordered](
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
) *LangParserSession[TObservation] {
	session := &LangParserSession[TObservation]{
		sourceFile: sourceFile,
		mapFn:      mapFn,
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

NOTE: mapFn can be zero if the TObservation is byte or rune. If TObservation is byte/rune, mapFn will not be used.
*/
func (l *LangParserSession[TObservation]) Reset(
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
) {
	if l.inUse.Load() {
		panic("Cannot reset an active lang-parser session.")
	}

	l.sourceFile = sourceFile
	l.mapFn = mapFn

	l.inUse.Store(false)
}

// ---------------------------------------------------------------- LANG PARSER

/* LangParser encapsulates the pipeline that transforms a source into an LST. */
type LangParser[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable] struct {
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind]

	lexer  *lexarch.Lexer[TObservation, TLexerState, TToken, TTokenRole]
	parser *syntaxa.SyntaxaParser[TObservation, TToken, TTokenRole, TNodeKind, TLexerState]

	lexingSessionCache *lexarch.LexerSession[TObservation, TLexerState, TToken, TTokenRole]

	destroyed atomic.Bool
}

/*
LangParserLexerCreateFromSpec creates a lexer according to the spec.

This is useful if clients want to delegate lexing creation without relying on the rest of LSpec's LangParser.
diagnosticFn is optional (nil disables); when set it is invoked per ruleset with NFA build stats before NFA-to-DFA.
*/
func LangParserLexerCreateFromSpec[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole comparable](
	scratchAllocationFn memarch.AllocationFn,
	maxLexerAutomatonMemory memcore.MemoryUnitBytes,
	nfaToDFAPipelineMinTemp, nfaToDFAPipelineMaxTemp memcore.MemoryUnitBytes,
	spec *LexerSpec[TObservation, TToken, TTokenRole, TLexerState],
) *lexarch.Lexer[TObservation, TLexerState, TToken, TTokenRole] {
	return lexarch.LexerCreate(
		spec.rulesets,
		spec.eofToken,
		scratchAllocationFn,
		maxLexerAutomatonMemory,
		nfaToDFAPipelineMinTemp,
		nfaToDFAPipelineMaxTemp,
		lexarch.ObservationCTXCreate(
			spec.observationFormatter,
			spec.observationDomain,
			spec.toBytes,
		),
		spec.compilerMode,
		spec.scanConfig,
	)
}

/* LangParserCreate constructs a language parser instance. */
func LangParserCreate[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	config *LangParserConfiguration[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
) *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind] {
	lexer := LangParserLexerCreateFromSpec(config.scratchAllocationFn, config.maxLexerAutomatonMemory, config.nfaToDFAPipelineMinTemp, config.nfaToDFAPipelineMaxTemp, config.spec.Lexer)

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
		config.spec.Parser.getAnalysis,
	)
	parser.SetDefaultSkips(config.spec.Parser.defaultSkipRoles...)
	parser.EnableTrace(config.collectParseTrace)
	if config.nodePoolGrowFn != nil {
		parser.SetNodePoolGrowFn(config.nodePoolGrowFn)
	}

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

/* SetCollectParseTrace toggles parse trace collection for subsequent parses (see WithCollectParseTrace). */
func (p *LangParser[TObs, TLexerState, TToken, TTokenRole, TNodeKind]) SetCollectParseTrace(enable bool) {
	p.parser.EnableTrace(enable)
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

parseStats may be nil. When non-nil, LexPretokenize and ParseOnly are set (pretokenize materialization
timing), Stream and Engine are filled from the parse context, and lexer-derived counters are written after parsing.
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
	parseStats *LangParseStats,
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

	var streamStatsPtr *syntaxa.ParseStreamStats
	var engineStatsPtr *syntaxa.ParseEngineStats
	if parseStats != nil {
		streamStatsPtr = &parseStats.Stream
		engineStatsPtr = &parseStats.Engine
	}

	parsingContext, sliceLexSession, err := buildSequentialParsingContext(
		langParser,
		session.sourceFile,
		session.mapFn,
		syntaxErrors,
		streamStatsPtr,
		engineStatsPtr,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	if parseStats != nil {
		lexStart := time.Now()
		if err2 := lexarch.LexerSessionEnsurePreTokenizedAll(langParser.lexer, sliceLexSession); err2 != nil {
			return nil, nil, syntaxErrors, err2
		}
		parseStats.LexPretokenize = time.Since(lexStart)
		n, ok := lexarch.LexerSessionPreTokenizedLexemeCount(sliceLexSession)
		parseStats.RawLexemeStreamLen = n
		parseStats.RawLexemeStreamOk = ok
	}

	parseStart := time.Now()
	rootNode, trace, err := syntaxa.SyntaxaParserParseWithContext(
		langParser.parser,
		parsingContext,
	)
	if parseStats != nil {
		parseStats.ParseOnly = time.Since(parseStart)
		parseStats.LexObservationSteps = lexarch.LexerScanStatsObservationSteps(langParser.lexer)
		parseStats.LSTNodeCount = parsingContext.Editor.CreatedCount()
		parseStats.FinalLexerNextTokenNumber = lexarch.LexerSessionNextTokenNumber(sliceLexSession)
	}

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

/*
LangParserLexScanStatsBind attaches optional lex scan counters to the parser’s lexer. Pass nil to detach.

Time complexity: O(1)
Space complexity: O(1)
*/
func LangParserLexScanStatsBind[
	TObservation cmp.Ordered,
	TLexerState,
	TToken,
	TTokenRole,
	TNodeKind comparable,
](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	stats *lexarch.LexScanStats,
) {
	ensureAlive(langParser)
	lexarch.LexerScanStatsBind(langParser.lexer, stats)
}

// ---------------------------------------------------------------- PRIVATE HELPERS

func buildSequentialParsingContext[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceFile string,
	mapFn func([]byte) ([]TObservation, error),
	syntaxErrors *syntaxa.SyntaxErrors[TObservation],
	streamStats *syntaxa.ParseStreamStats,
	engineStats *syntaxa.ParseEngineStats,
) (
	*syntaxa.ExecRuleContext[TObservation, TToken, TTokenRole, TLexerState, TNodeKind],
	*lexarch.LexerSession[TObservation, TLexerState, TToken, TTokenRole],
	error,
) {
	sourceInput, err := getSourceInput(sourceFile, mapFn)
	if err != nil {
		return nil, nil, fmt.Errorf("could not decode source-file: %w", err)
	}

	lexingSession := getLexerSession(langParser, sourceInput)

	langParser.parser.SetNodePoolPrefill(
		nodePoolPrefillFromObservationCount(len(sourceInput), langParser.config.nodePoolPrefill),
	)

	parsingContext := syntaxa.BuildExecRuleContextFromLexerSession(
		langParser.parser,
		langParser.lexer,
		lexingSession,
		syntaxErrors,
		streamStats,
		engineStats,
	)

	return parsingContext, lexingSession, nil
}

func getLexerSession[TObservation cmp.Ordered, TLexerState, TToken, TTokenRole, TNodeKind comparable](
	langParser *LangParser[TObservation, TLexerState, TToken, TTokenRole, TNodeKind],
	sourceInput []TObservation,
) *lexarch.LexerSession[TObservation, TLexerState, TToken, TTokenRole] {
	if langParser.lexingSessionCache == nil {
		switch langParser.config.spec.Lexer.positionMode {
		case lexerSpecPositionModeRuneFast:
			inputRunes, ok := any(sourceInput).([]rune)
			if !ok {
				panic("WithRunePositionTrackingFast requires rune observations")
			}
			sessionRunes := lexarch.LexerSessionCreateRuneFast[TLexerState, TToken, TTokenRole](
				langParser.config.spec.Lexer.initialState,
				inputRunes,
				langParser.config.spec.Lexer.runeTabWidth,
			)
			session, ok := any(sessionRunes).(*lexarch.LexerSession[TObservation, TLexerState, TToken, TTokenRole])
			if !ok {
				panic("rune fast session type mismatch")
			}
			langParser.lexingSessionCache = session
			return session
		case lexerSpecPositionModeByteFast:
			inputBytes, ok := any(sourceInput).([]byte)
			if !ok {
				panic("WithBytePositionTrackingFast requires byte observations")
			}
			sessionBytes := lexarch.LexerSessionCreateByteFast[TLexerState, TToken, TTokenRole](
				langParser.config.spec.Lexer.initialState,
				inputBytes,
			)
			session, ok := any(sessionBytes).(*lexarch.LexerSession[TObservation, TLexerState, TToken, TTokenRole])
			if !ok {
				panic("byte fast session type mismatch")
			}
			langParser.lexingSessionCache = session
			return session
		default:
			session := lexarch.LexerSessionCreate[TObservation, TLexerState, TToken, TTokenRole](
				langParser.config.spec.Lexer.initialState,
				sourceInput,
				langParser.config.spec.Lexer.newlineDetect,
				langParser.config.spec.Lexer.columnAdvanceFn,
			)
			langParser.lexingSessionCache = session
			return session
		}
	}

	langParser.lexingSessionCache.Reset(sourceInput, langParser.config.spec.Lexer.initialState)
	return langParser.lexingSessionCache
}

func nodePoolPrefillFromObservationCount(observationCount int, override int) int {
	if override > 0 {
		return override
	}

	const minHint = 128
	const maxHint = 262144

	if observationCount <= 0 {
		return minHint
	}

	h := observationCount / 16
	if h < minHint {
		return minHint
	}
	if h > maxHint {
		return maxHint
	}
	return h
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
