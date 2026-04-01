package langspec

import (
	"syntaxa"
	"time"
)

/*
LangParseStats is filled by LangParserParseFile when the stats pointer is non-nil.

LexPretokenize covers one-time session token prefill/warmup before parse.
ParseOnly is SyntaxaParserParseWithContext wall time after that.

Stream holds raw peek/consume counts at the lexer boundary. RawLexemeStreamLen is the
materialized token count captured before parse and is not affected by parser restores.
LexObservationSteps counts lexer observation work gathered during lexing.

Engine holds Syntaxa parse-engine counters (rule attempts, choice dispatch, recovery, LST pool).
LSTNodeCount is len(Editor.created) after parse.
*/
type LangParseStats struct {
	LexPretokenize time.Duration
	ParseOnly      time.Duration
	Stream         syntaxa.ParseStreamStats
	Engine         syntaxa.ParseEngineStats

	RawLexemeStreamLen        int
	RawLexemeStreamOk         bool
	FinalLexerNextTokenNumber int
	LexObservationSteps       uint64
	LSTNodeCount              int
}
