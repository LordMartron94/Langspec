package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"langspec"
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

PRAGMA {
	lspec { library = true; }
}
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

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
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

	for _, state := range res.CompiledLexerSpec.SortedStateKeys() {
		if state == "M__INITIAL" {
			t.Fatalf("host lexer should not auto-materialize imported lexer states")
		}
	}
}

func TestImportExportImportedSyntaxErrorPropagatesWithContext(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_bad_syntax.lspec")
	mainPath := filepath.Join(tmpDir, "main_bad_syntax_import.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `
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
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
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

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err == nil {
		t.Fatalf("expected compile failure for imported syntax error")
	}
	if res == nil || len(res.ImportedDiagnostics) == 0 {
		t.Fatalf("expected imported diagnostics for imported syntax error")
	}
	foundSyntax := false
	for _, diag := range res.ImportedDiagnostics {
		if diag.Alias == "M" && diag.Code == "SYNTAX" {
			foundSyntax = true
			break
		}
	}
	if !foundSyntax {
		t.Fatalf("expected imported SYNTAX diagnostic for alias M")
	}
}

func TestImportExportImportedValidationErrorPropagatesWithContext(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_bad_validation.lspec")
	mainPath := filepath.Join(tmpDir, "main_bad_validation_import.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
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
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
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

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err == nil {
		t.Fatalf("expected compile failure for imported validation error")
	}
	if res == nil || len(res.ImportedDiagnostics) == 0 {
		t.Fatalf("expected imported diagnostics for imported validation error")
	}
	foundProgramRequired := false
	for _, diag := range res.ImportedDiagnostics {
		if diag.Alias == "M" && diag.Code == "V_PAR004" {
			foundProgramRequired = true
			break
		}
	}
	if !foundProgramRequired {
		t.Fatalf("expected imported V_PAR004 diagnostic for alias M")
	}
}

func TestImportExportLibraryPragmaAllowsMissingProgram(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_library_missing_program.lspec")
	mainPath := filepath.Join(tmpDir, "main_library_import.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
PRAGMA {
	lspec { library = true; }
}
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
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
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { word };
	rule PROGRAM -> ProgramNode { using M.Item };
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("expected compile success for library module missing PROGRAM: %v", err)
	}
}

func TestImportExportLibraryProgramRuleForbidden(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_library_with_program.lspec")
	mainPath := filepath.Join(tmpDir, "main_library_with_program_import.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
PRAGMA {
	lspec { library = true; }
}
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	rule PROGRAM -> ProgramNode { virtual TokWord };
	export rule Item -> ItemNode { virtual TokWord };
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
	rule PROGRAM -> ProgramNode { virtual TokWord };
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err == nil {
		t.Fatalf("expected compile failure for library imported module defining PROGRAM")
	}
	if res == nil || len(res.ImportedDiagnostics) == 0 {
		t.Fatalf("expected imported diagnostics for library PROGRAM violation")
	}

	foundProgramViolation := false
	for _, diag := range res.ImportedDiagnostics {
		if diag.Alias == "M" && diag.Code == "V_PAR004" {
			foundProgramViolation = true
			break
		}
	}
	if !foundProgramViolation {
		t.Fatalf("expected imported V_PAR004 for library PROGRAM rule violation")
	}
}

func TestImportExportNonLibraryMissingProgramStillFails(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_non_library_missing_program.lspec")
	mainPath := filepath.Join(tmpDir, "main_non_library_import.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
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
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
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

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err == nil {
		t.Fatalf("expected compile failure for non-library imported module missing PROGRAM")
	}
	if res == nil || len(res.ImportedDiagnostics) == 0 {
		t.Fatalf("expected imported diagnostics for non-library missing PROGRAM failure")
	}
	foundProgramRequired := false
	for _, diag := range res.ImportedDiagnostics {
		if diag.Alias == "M" && diag.Code == "V_PAR004" {
			foundProgramRequired = true
			break
		}
	}
	if !foundProgramRequired {
		t.Fatalf("expected imported V_PAR004 for non-library module missing PROGRAM")
	}
}

func TestImportExportLibraryPragmaAllowsUnresolvedReferences(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_library_unresolved.lspec")
	mainPath := filepath.Join(tmpDir, "main_library_unresolved_import.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---
PRAGMA {
	lspec { library = true; }
}
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	rule Hidden -> HiddenNode { MissingRule };
	export rule Item -> ItemNode { virtual TokWord };
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
	rule PROGRAM -> ProgramNode { using M.Item };
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		if res != nil && len(res.ImportedDiagnostics) > 0 {
			var msgs []string
			for _, d := range res.ImportedDiagnostics {
				msgs = append(msgs, d.Code+": "+d.Message)
			}
			t.Fatalf("expected unresolved checks to be relaxed in library mode; imported diagnostics: %s", strings.Join(msgs, "; "))
		}
		t.Fatalf("expected compile success with library unresolved references allowed: %v", err)
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

PRAGMA {
	lspec { library = true; }
}
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

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
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

PRAGMA {
	lspec { library = true; }
}
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

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
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

func TestImportExportTemplateOnlyPullsImportedLexerRules(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_tpl_tokens.lspec")
	mainPath := filepath.Join(tmpDir, "main_tpl_tokens.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---

PRAGMA {
	lspec { library = true; }
}
LEX {
	state INITIAL {
		TokTplWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	export template EmitWord($r : Rule) { virtual TokTplWord }
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---

IMPORT {
	"` + modulePath + `" as M;
}
LEX {
	state INITIAL {
		TokMain -> word: ` + "`[0-9]+`" + `;
		TokTplWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { word };
	rule PROGRAM -> ProgramNode {
		using M.EmitWord(PROGRAM)
	};
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil || res.CompiledLexerSpec == nil {
		t.Fatalf("compile result missing lowered artifacts")
	}

	if res.CompiledSymbols.TokenID("TokTplWord") == 0 {
		t.Fatalf("expected host token TokTplWord in compiled symbols")
	}
	if res.CompiledSymbols.TokenID("M__TokTplWord") != 0 {
		t.Fatalf("imported namespaced token should not be materialized")
	}
	for _, state := range res.CompiledLexerSpec.SortedStateKeys() {
		if state == "M__INITIAL" {
			t.Fatalf("imported lexer state should not be generated")
		}
	}
}

func TestImportExportCompiledSymbolsNamespaceImportedCollisions(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modAPath := filepath.Join(tmpDir, "module_a.lspec")
	modBPath := filepath.Join(tmpDir, "module_b.lspec")
	mainPath := filepath.Join(tmpDir, "main_collision.lspec")

	moduleA := `--- "ModuleA" v1.0.0 | lspec v1.0.0 ---

PRAGMA {
	lspec { library = true; }
}
LEX {
	state INITIAL {
		TokShared -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	export rule SharedRule -> SharedNode { virtual TokShared };
}`

	moduleB := `--- "ModuleB" v1.0.0 | lspec v1.0.0 ---

PRAGMA {
	lspec { library = true; }
}
LEX {
	state INITIAL {
		TokShared -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	export rule SharedRule -> SharedNode { virtual TokShared };
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---

IMPORT {
	"` + modAPath + `" as A;
	"` + modBPath + `" as B;
}
LEX {
	state INITIAL {
		TokMain -> word: ` + "`[0-9]+`" + `;
		TokShared -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { word };
	rule PROGRAM -> ProgramNode {
		using A.SharedRule
		using B.SharedRule
	};
}`

	if err := os.WriteFile(modAPath, []byte(moduleA), 0644); err != nil {
		t.Fatalf("write module A spec: %v", err)
	}
	if err := os.WriteFile(modBPath, []byte(moduleB), 0644); err != nil {
		t.Fatalf("write module B spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil {
		t.Fatalf("compile result missing compiled symbols")
	}

	if res.CompiledSymbols.TokenID("TokShared") == 0 {
		t.Fatalf("expected host-owned token TokShared to be compiled")
	}
	if res.CompiledSymbols.TokenID("A__TokShared") != 0 || res.CompiledSymbols.TokenID("B__TokShared") != 0 {
		t.Fatalf("imported namespaced tokens should not be materialized")
	}

	aNode := res.CompiledSymbols.NodeKindID("A__SharedNode")
	bNode := res.CompiledSymbols.NodeKindID("B__SharedNode")
	if aNode == 0 || bNode == 0 || aNode == bNode {
		t.Fatalf("expected non-colliding namespaced imported nodes A__SharedNode and B__SharedNode")
	}
}

func TestImportExportUsingRuleWithSiblingReferencesBuildsLocalGrammarLabels(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "module_std_like.lspec")
	mainPath := filepath.Join(tmpDir, "main_std_like.lspec")

	module := `--- "Module" v1.0.0 | lspec v1.0.0 ---

PRAGMA {
	lspec { library = true; }
}
PATTERN {
	export pat_TokKWGodebug : "godebug";
	export pat_TokIdent : ['a'..'z']+;
	export pat_Equals : '=';
}
LEX {
	state INITIAL {
		TokKWGodebug -> structural: pat_TokKWGodebug;
		TokIdent -> structural: pat_TokIdent;
		TokEquals -> structural: pat_Equals;
	}
}
PARSE {
	export rule GODEBUG_STATEMENT -> NodeGodebugStatement {
		virtual TokKWGodebug
		GODEBUG_REFERENCE
	}
	export rule GODEBUG_REFERENCE -> NodeGodebugReference {
		NodeGodebugKey : TokIdent
		virtual TokEquals
		NodeGodebugValue : TokIdent
	}
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---

IMPORT {
	"` + modulePath + `" as M;
}
LEX {
	state INITIAL {
		TokOwnWhitespace -> own: ` + "`\\s+`" + `;
		TokKWGodebug -> structural: using M.pat_TokKWGodebug;
		TokIdent -> structural: using M.pat_TokIdent;
		TokEquals -> structural: using M.pat_Equals;
	}
}
PARSE {
	IGNORE { own };
	rule PROGRAM -> ProgramNode {
		(virtual TokKWGodebug)?
		(virtual TokIdent)?
		(virtual TokEquals)?
		GODEBUG_STATEMENT*
		GODEBUG_REFERENCE*
	};
	rule GODEBUG_STATEMENT -> transparent gr_GODEBUG_STATEMENT { using M.GODEBUG_STATEMENT };
	rule GODEBUG_REFERENCE -> transparent gr_GODEBUG_REFERENCE { using M.GODEBUG_REFERENCE };
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module spec: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write main spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil || res.CompiledParserSpec == nil {
		t.Fatalf("compile result missing compiled artifacts")
	}

	if res.CompiledSymbols.TokenID("TokKWGodebug") == 0 || res.CompiledSymbols.TokenID("TokIdent") == 0 || res.CompiledSymbols.TokenID("TokEquals") == 0 {
		t.Fatalf("expected host tokens to satisfy imported rule lexical obligations")
	}
	if res.CompiledSymbols.TokenID("M__TokKWGodebug") != 0 || res.CompiledSymbols.TokenID("M__TokIdent") != 0 {
		t.Fatalf("imported namespaced tokens should not be materialized")
	}
}

func TestEmbedImportMergesLexerNamespaceAndInjectsHandoff(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "sql_embed.lspec")
	mainPath := filepath.Join(tmpDir, "main_embed.lspec")

	module := `--- "SQL" v1.0.0 | lspec v1.0.0 ---
LEX {
	state INITIAL {
		TokSQLWord -> sql: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	rule PROGRAM -> NodeSQLProgram { virtual TokSQLWord };
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---
IMPORT {
	embed "` + modulePath + `" as SQL;
}
LEX {
	state INITIAL {
		TokOpen -> delim: ` + "`BEGINSQL`" + `;
		TokClose -> delim: ` + "`ENDSQL`" + `;
		TokHostWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { delim word };
	rule PROGRAM -> ProgramNode {
		embed SQL nest TokOpen TokClose
	};
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write embed module: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write host spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil || res.CompiledLexerSpec == nil {
		t.Fatalf("compile result missing compiled artifacts")
	}
	if res.CompiledSymbols.TokenID("SQL::TokSQLWord") == 0 {
		t.Fatalf("expected namespaced embedded token SQL::TokSQLWord in compiled symbols")
	}
	hasEmbedState := false
	for _, state := range res.CompiledLexerSpec.SortedStateKeys() {
		if state == "SQL::INITIAL" {
			hasEmbedState = true
			break
		}
	}
	if !hasEmbedState {
		t.Fatalf("expected namespaced embedded lexer state SQL::INITIAL")
	}

	hostRules := langspec.LexerRulesetGetRules(res.CompiledLexerSpec.Ruleset("INITIAL"))
	foundEntryPush := false
	for _, rule := range hostRules {
		if rule.Token != res.CompiledSymbols.TokenID("TokOpen") {
			continue
		}
		if rule.StackKind == langspec.LexerStackOpPush && len(rule.StackStates) > 0 && rule.StackStates[0] == "SQL::INITIAL" {
			foundEntryPush = true
			break
		}
	}
	if !foundEntryPush {
		t.Fatalf("expected TokOpen host rule to push SQL::INITIAL")
	}

	embedRules := langspec.LexerRulesetGetRules(res.CompiledLexerSpec.Ruleset("SQL::INITIAL"))
	foundExitPop := false
	for _, rule := range embedRules {
		if rule.Token != res.CompiledSymbols.TokenID("TokClose") {
			continue
		}
		if rule.StackKind == langspec.LexerStackOpPop && rule.StackPopAmount == 1 {
			foundExitPop = true
			break
		}
	}
	if !foundExitPop {
		t.Fatalf("expected injected TokClose pop(1) rule in SQL::INITIAL")
	}
}

func TestEmbedStatementRequiresEmbedImportAlias(t *testing.T) {
	t.Parallel()

	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	tmpDir := t.TempDir()
	modulePath := filepath.Join(tmpDir, "mod_plain_import.lspec")
	mainPath := filepath.Join(tmpDir, "main_embed_alias_error.lspec")

	module := `--- "M" v1.0.0 | lspec v1.0.0 ---
LEX {
	state INITIAL {
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	rule PROGRAM -> NodeProg { virtual TokWord };
}`

	main := `--- "Main" v1.0.0 | lspec v1.0.0 ---
IMPORT {
	"` + modulePath + `" as M;
}
LEX {
	state INITIAL {
		TokOpen -> delim: ` + "`BEGIN`" + `;
		TokClose -> delim: ` + "`END`" + `;
		TokWord -> word: ` + "`[a-zA-Z]+`" + `;
	}
}
PARSE {
	IGNORE { delim word };
	rule PROGRAM -> ProgramNode { embed M nest TokOpen TokClose };
}`

	if err := os.WriteFile(modulePath, []byte(module), 0644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	if err := os.WriteFile(mainPath, []byte(main), 0644); err != nil {
		t.Fatalf("write host spec: %v", err)
	}

	res, err := dsl.LangSpecCompilerCompile[dsl.LangSpecParserNodeKind](compiler, mainPath)
	if err == nil {
		t.Fatalf("expected compile failure when embed uses non-embed import alias")
	}
	if res == nil || res.ValidationEntries == nil {
		t.Fatalf("expected validation entries")
	}
	found := false
	for _, stage := range res.ValidationEntries.Results {
		for _, entry := range stage.Entries {
			if entry.Code == "V_IMP009" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Fatalf("expected V_IMP009 for embed alias misuse")
	}
}
