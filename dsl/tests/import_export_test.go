package tests

import (
	"os"
	"path/filepath"
	"testing"

	"langspec/dsl"
)

func TestImportExportUsingRuleAndPair(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module.lspec")
	mainPath := filepath.Join(tmpDir, "main.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---

LEX {
	state INITIAL {
		TokOpen -> delim: ` + "`\\{`" + `;
		TokClose -> delim: ` + "`\\}`" + `;
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
		TokUnused -> hidden: ` + "`_+`" + `;
	}
}
PARSE {
	IGNORE { hidden };
	export pair Braces TokOpen TokClose;
	export rule Item -> ItemNode { virtual TokWord };
	export template ItemTpl($r : Rule) { $r }
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---

IMPORT {
	"` + modulePath + `" as M;
}
LEX {
	state INITIAL {
		TokOpen -> delim: ` + "`\\{`" + `;
		TokClose -> delim: ` + "`\\}`" + `;
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { delim word };
	rule PROGRAM -> ProgramNode {
		nest using M.Braces {
			using M.Item
			using M.ItemTpl(PROGRAM)
		}
	};
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil || res.CompiledLexerSpec == nil {
		t.Fatalf("compile result missing lowered artifacts")
	}

	if tokUnused := res.CompiledSymbols.TokenID("TokUnused"); tokUnused != 0 {
		t.Fatalf("strict import token filtering failed: TokUnused should not be in compiled symbols")
	}

	states := res.CompiledLexerSpec.SortedStateKeys()
	foundImportedState := false
	for _, state := range states {
		if state == "M__INITIAL" {
			foundImportedState = true
			break
		}
	}
	if !foundImportedState {
		t.Fatalf("expected imported lexer state M__INITIAL to be generated")
	}
}

func TestImportExportValidationForExternalTemplateCallErrors(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_errors.lspec")
	mainPath := filepath.Join(tmpDir, "main_errors.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---

LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	export rule Item -> ItemNode { virtual TokWord };
	export template ItemTpl($r : Rule) { $r }
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---

IMPORT {
	"` + modulePath + `" as M;
}
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { word };
	rule PROGRAM -> ProgramNode {
		using M.ItemTpl
		using M.Item(TokWord)
		using M.Unknown
	};
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
	if err == nil {
		t.Fatalf("expected validation failure for invalid external template usage")
	}
	if res == nil || res.ValidationEntries == nil {
		t.Fatalf("expected validation entries for invalid external template usage")
	}

	foundTemplateCallShape := false
	foundTemplateCallCategory := false
	foundUnresolvedExport := false
	for _, stage := range res.ValidationEntries.Results {
		for _, entry := range stage.Entries {
			switch entry.Code {
			case "V_IMP006":
				if entry.Message == "template 'M.ItemTpl' must be invoked as using M.ItemTpl(...)" {
					foundTemplateCallShape = true
				} else {
					foundTemplateCallCategory = true
				}
			case "V_IMP005":
				foundUnresolvedExport = true
			}
		}
	}
	if !foundTemplateCallShape {
		t.Fatalf("expected V_IMP006 for missing template call syntax")
	}
	if !foundUnresolvedExport {
		t.Fatalf("expected V_IMP005 for unresolved exported symbol")
	}
	if !foundTemplateCallCategory {
		t.Fatalf("expected V_IMP006 for invoking non-template with call args")
	}
}

func TestImportExportLexUsingPatternValidation(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_pattern.lspec")
	mainPath := filepath.Join(tmpDir, "main_pattern.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---

PATTERN {
	export word_pat : ` + "`[a-zA-Z]+`" + `;
}
LEX {
	state INITIAL {
		TokWord -> word: word_pat;
	}
}
PARSE {
	export rule Item -> ItemNode { virtual TokWord };
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---

IMPORT {
	"` + modulePath + `" as M;
}
LEX {
	state INITIAL {
		TokWord -> word: using M.word_pat;
		TokBadRule -> word: using M.Item;
		TokMissing -> word: using M.MissingPat;
	}
}
PARSE {
	IGNORE { word };
	rule PROGRAM -> ProgramNode { virtual TokWord };
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
	if err == nil {
		t.Fatalf("expected validation failure for invalid lex using references")
	}
	if res == nil || res.ValidationEntries == nil {
		t.Fatalf("expected validation entries for invalid lex using references")
	}

	foundWrongCategory := false
	foundUnresolved := false
	for _, stage := range res.ValidationEntries.Results {
		for _, entry := range stage.Entries {
			switch entry.Code {
			case "V_IMP006":
				if entry.Message == "lexer using requires an exported pattern, got 'M.Item'" {
					foundWrongCategory = true
				}
			case "V_IMP005":
				if entry.Message == "unresolved exported symbol 'M.MissingPat'" {
					foundUnresolved = true
				}
			}
		}
	}
	if !foundWrongCategory {
		t.Fatalf("expected V_IMP006 for using non-pattern export in lex section")
	}
	if !foundUnresolved {
		t.Fatalf("expected V_IMP005 for unresolved external pattern in lex section")
	}
}
