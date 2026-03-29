package langspec

import (
	"syntaxa"
	"time"
)

/*
LangParseStats is filled by LangParserParseFile when the stats pointer is non-nil.

LexPretokenize covers pretokenized lexeme materialization (LexerSessionEnsurePreTokenizedAll).
ParseOnly is SyntaxaParserParseWithContext wall time after that.

Stream holds raw peek/consume counts at the lexer boundary. RawLexemeStreamLen is len(preTokens)
for the full input, read immediately after LexerSessionEnsurePreTokenizedAll (before parse), so it
is not affected by parser restore clearing or rebuilding the pretokenized cache. LexObservationSteps
is copied from the lexer’s bound LexScanStats after parse, if any.

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
