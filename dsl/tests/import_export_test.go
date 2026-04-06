package tests

import (
	"langspec"
	"os"
	"path/filepath"
	"strings"
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
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
		TokMain -> word: ` + "`[a-zA-Z]+`" + `;
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil || res.CompiledLexerSpec == nil {
		t.Fatalf("compile result missing lowered artifacts")
	}

	importedTok := res.CompiledSymbols.TokenID("M__TokTplWord")
	if importedTok == 0 {
		t.Fatalf("expected namespaced imported template token M__TokTplWord in compiled symbols")
	}

	rs := res.CompiledLexerSpec.Ruleset("M__INITIAL")
	if len(langspec.LexerRulesetGetRules(rs)) == 0 {
		t.Fatalf("expected namespaced imported lexer ruleset M__INITIAL to contain template token rules")
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
		TokMain -> word: ` + "`[a-zA-Z]+`" + `;
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

	res, err := dsl.LangSpecCompilerCompile(compiler, mainPath)
	if err != nil {
		logCompileFailureDiagnostics(t, res)
		t.Fatalf("LangSpecCompilerCompile failed: %v", err)
	}
	if res == nil || res.CompiledSymbols == nil {
		t.Fatalf("compile result missing compiled symbols")
	}

	aTok := res.CompiledSymbols.TokenID("A__TokShared")
	bTok := res.CompiledSymbols.TokenID("B__TokShared")
	if aTok == 0 || bTok == 0 || aTok == bTok {
		t.Fatalf("expected non-colliding namespaced imported tokens A__TokShared and B__TokShared")
	}

	aNode := res.CompiledSymbols.NodeKindID("A__SharedNode")
	bNode := res.CompiledSymbols.NodeKindID("B__SharedNode")
	if aNode == 0 || bNode == 0 || aNode == bNode {
		t.Fatalf("expected non-colliding namespaced imported nodes A__SharedNode and B__SharedNode")
	}
}
