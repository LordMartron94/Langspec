package dsl

import (
	"langspec/validation"
	"testing"
)

func TestValidationFailsCompilation_warningsAsErrors(t *testing.T) {
	warnEntry := validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{
		Code:     "W",
		Message:  "warn",
		Severity: validation.VALIDATION_SEVERITY_WARNING,
	}
	errEntry := validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{
		Code:     "E",
		Message:  "err",
		Severity: validation.VALIDATION_SEVERITY_ERROR,
	}

	entries := &validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{
		Results: []validation.StageValidationResult[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{
			{StageName: "t", Entries: []validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{warnEntry}},
		},
	}
	if !validationFailsCompilation(entries, true) {
		t.Fatal("expected failure when warnings_as_errors and WARNING present")
	}
	if validationFailsCompilation(entries, false) {
		t.Fatal("expected success when warnings ignored and only WARNING present")
	}

	entriesErr := &validation.ValidationEntries[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{
		Results: []validation.StageValidationResult[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{
			{StageName: "t", Entries: []validation.ValidationEntry[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecParserNodeKind]{errEntry}},
		},
	}
	if !validationFailsCompilation(entriesErr, false) {
		t.Fatal("expected failure on ERROR when warnings_as_errors is false")
	}
}
