package tests

import (
	"os"
	"path/filepath"
	"testing"

	"langspec/dsl"
)

func TestLexTokenMayBeDeclaredAcrossStates(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "multistate_token_reuse.lspec")

	spec := `--- "TokenReuse" v1.0.0 | lspec v1.0.0 ---

PATTERN {
	pat_ws  : (['\t', ' '])+;
	pat_nl  : (['\n', '\r'])+;
	pat_id  : ['A'..'Z', '_', 'a'..'z'] (['0'..'9', 'A'..'Z', '_', 'a'..'z'])*;
	pat_eof : [];
}

LEX {
	state INITIAL {
		1 TokIdentifier -> structural : pat_id;
		0 TokWhitespace -> whitespace : pat_ws [push(INTERPOLATION)];
		0 TokNewLine    -> structural : pat_nl;
		0 TokEOF        -> structural : pat_eof %% EOF=true %%;
	}

	state INTERPOLATION {
		1 TokIdentifier -> structural : pat_id;
		0 TokWhitespace -> whitespace : pat_ws [push(INTERPOLATION)];
	}
}

PARSE {
	IGNORE { whitespace };
	rule PROGRAM -> NodeProgram {
		(virtual TokIdentifier | virtual TokNewLine)*
		virtual TokEOF
	}
}`

	if err := os.WriteFile(specPath, []byte(spec), 0644); err != nil {
		t.Fatalf("write test spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile(compiler, specPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("expected multi-state token reuse to be valid, got: %v", err)
	}
	if res == nil {
		t.Fatalf("expected compile result")
	}
}

func TestLexTokenReuseAcrossStatesStillEnforcesConsistency(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	specPath := filepath.Join(tmpDir, "multistate_token_conflict.lspec")

	spec := `--- "TokenConflict" v1.0.0 | lspec v1.0.0 ---

PATTERN {
	pat_a   : "a";
	pat_b   : "b";
	pat_eof : [];
}

LEX {
	state INITIAL {
		1 TokShared -> structural : pat_a;
		0 TokEOF    -> structural : pat_eof %% EOF=true %%;
	}

	state INNER {
		1 TokShared -> structural : pat_b;
	}
}

PARSE {
	rule PROGRAM -> NodeProgram {
		(virtual TokShared)*
		virtual TokEOF
	}
}`

	if err := os.WriteFile(specPath, []byte(spec), 0644); err != nil {
		t.Fatalf("write test spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile(compiler, specPath)
	if err == nil {
		t.Fatalf("expected validation failure for inconsistent multi-state token reuse")
	}
	if res == nil || res.ValidationEntries == nil {
		t.Fatalf("expected validation entries for inconsistent multi-state token reuse")
	}

	foundConsistencyError := false
	for _, stage := range res.ValidationEntries.Results {
		for _, entry := range stage.Entries {
			if entry.Code == "V_LEX011" {
				foundConsistencyError = true
				break
			}
		}
		if foundConsistencyError {
			break
		}
	}
	if !foundConsistencyError {
		t.Fatalf("expected V_LEX011 for inconsistent token definition across lexer states")
	}
}
