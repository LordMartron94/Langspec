# LangSpec

LangSpec is a language specification and parsing ecosystem. It provides a meta-language (the LangSpec DSL) for describing lexers and grammars, plus tooling to parse source files and to generate editor integrations (e.g. Sublime Text syntax definitions). The `.lspec` grammar itself does **not** include logic for specifying validation—validation is semantic and would require imperative programming, so it is implemented on the Go side (see package **`validation`** and compiler validation stages).

## Overview

LangSpec separates **language semantics** from **execution mechanics**. You define a language in a `.lspec` file using the DSL; the DSL compiler turns that into runtime structures (`LexerSpec`, `ParserSpec`, `LangSpec`) that drive the parsing pipeline:

```text
DSL source (.lspec)  →  LangSpecCompilerCompile  →  LangSpecCompileResult
                                                           ↓
                                              CompiledLexerSpec, CompiledParserSpec
                                                           ↓
                                              LangSpecCreate  →  LangParser  →  LST
```

The core pipeline is: **raw source → lexing → parsing → LST** (Lossless Syntax Tree). The root package provides `LangParser`, `LangSpecCreate`, and file-based parsing; the `dsl` package provides the DSL compiler that consumes `.lspec` files and produces specs usable by the core.

## Design Philosophy

- **Data vs. execution:** Language definitions are data (`.lspec`); the engine is generic. No domain logic in the core.
- **Self-hosting:** The LangSpec DSL is parsed by the same LangSpec engine, so the meta-language and user languages share one pipeline and tooling.
- **Stable contracts:** Public APIs (compiler result, parser config, spec types) are explicit and documented. Breaking changes are avoided once shared.
- **Separation of concerns:** Lexer spec, parser spec, and editor IR are independent; validation is not part of the DSL grammar and lives in Go. The DSL compiler assembles lexer and parser for the meta-language and runs validation stages after parsing.

## Performance Characteristics

- **Lexer:** NFA-to-DFA compilation with configurable memory bounds; optional streaming for large inputs.
- **Parser:** Grammar-driven with optional Pratt expression parsing; skip roles for whitespace/comments.
- **Compiler:** Single-pass over LST to produce specs; diagnostic output is optional and sink-directed.
- **Memory:** Bounded by `LangParserConfiguration` (max lexer automaton memory, NFA/DFA temp). Streaming config limits buffered observations.

## Integration

```text
langspec (root)
    ├── dsl              (DSL compiler: .lspec → CompiledLangSpec)
    │   ├── dsl/editor   (Sublime syntax for the DSL itself)
    │   └── dsl/generator (grammar/lexer → .lspec emitter)
    ├── editor           (push-down automaton IR for syntax highlighting)
    │   └── editor/sublime (IR → Sublime YAML)
    ├── toolchain        (tool-specific helpers, e.g. Sublime config)
    └── validation       (LST validation stages and reporting)
```

Dependencies: `lexarch` (lexing), `syntaxa` (grammar/LST), `autarch/pattern` (regex/Regula), `foundation` (system, domain).

---

## Packages

### Root package (`langspec`)

Public API for parsing and specs:

- **`LangSpecCreate`** — Builds a `LangSpec` from a `LexerSpec` and a `ParserSpec`.
- **`LangParserCreate`** — Builds a `LangParser` from a `LangParserConfiguration`.
- **`LangParserParseFile`** — Parses a file; returns parse trace, root LST node, syntax errors, and error.
- **`LangParserSessionCreate`** / **`Reset`** — Session for file path and optional mapping (e.g. runes); supports streaming.
- **`LangParserDestroy`** — Must be called to release parser resources.
- **`LexerSpecCreate`**, **`WithRuleset`** — Lexer specification.
- **`ParserSpecCreate`**, **`WithSkipRoles`**, **`WithErrorHook`** — Parser specification.
- **`LangParserConfigurationCreate`**, **`WithMaxLexerAutomatonMemory`**, etc. — Runtime and memory config.

### Package `dsl`

The LangSpec DSL and its compiler.

- **`LangSpecCompilerConfigurationCreate`** — Takes scratch allocation function and optional stage reporter. Use **`WithDiagnosticSink`** to set a sink (e.g. `DefaultLangSpecDiagnosticSink()`) for human-readable diagnostics.
- **`LangSpecCompilerCreate`** / **`LangSpecCompilerDestroy`** — Compiler lifecycle.
- **`LangSpecCompilerCompile(compiler, sourceFile)`** — Compiles a `.lspec` file. Returns **`LangSpecCompileResult`** (RootNode, Trace, SyntaxErrors, ValidationEntries, CompiledLexerSpec, CompiledParserSpec, LanguageName, LanguageVersion, CompiledGrammarPackage, CompiledToolPragmas, EOFToken) and an error. Result is populated even when there are syntax or validation errors; use `SyntaxErrors.HasErrors()` and validation severity to decide.
- **`LangSpecCompilerLexingRuleSet`**, **`LangSpecCompilerGrammarPackage`**, **`LangSpecCompilerScopeMap`** — Accessors for lexer rules, grammar package, and scope map (e.g. for codegen or editor).
- **`LangSpecCompilerDebugLexemes`**, **`LangSpecCompilerDebugGrammar`**, **`LangSpecCompilerDebugResult`** — Debug helpers when a diagnostic sink is set.

Compilation flow: read `.lspec` → parse with LangSpec’s own parser → validate LST (optional stages) → compile tree to `CompiledLangSpec` and expose via `LangSpecCompileResult`.

### Package `dsl/editor`

Editor integration for the LangSpec DSL itself (e.g. Sublime):

- **`BuildSublimeSyntaxForDSL(compiler, syntaxFile)`** — Writes a Sublime Text syntax definition for `.lspec` files using the compiler’s lexer and grammar. Sublime generation is driven by **configuration** (scope map, context producer, overrides). Complex overrides (e.g. line comment, block comment, regex literal) are **not** expressed in the configuration data—they are implemented by the compiler the client provides (e.g. `BuildEditorOverrideProducer` and token-specific override functions). See **Sublime syntax generation** below.

### Package `editor`

Generic editor IR (push-down automaton) for syntax highlighting:

- **`PushDownAutomatonIR`** — Stack-based states and rules (push/pop/set/match). Built from a syntaxa `GrammarPackage` and lexarch `LexingRuleset` via **`PushDownAutomatonIRCreate`** and config (scope provider, token formatter, overrides).
- Delimited tokens (open/close) map to regions; overrides allow custom rules and nest sequences.

### Package `editor/sublime`

- **`SublimeTextGenerateSyntaxFile`** — Generates Sublime Text YAML syntax from the editor IR.

### Sublime syntax generation

Sublime Text syntax is generated from the editor IR, which is built from a **GrammarPackage** and **LexingRuleset** plus **configuration**:

- **Scope provider / context producer** — Maps tokens and grammar nodes to Sublime scope strings (e.g. `comment.line`, `string.quoted.double`). Configuration supplies a base scope per token and optional per-node bindings (node kind → scopes, and optionally token-specific scopes within a node).
- **Token formatter** — Used for rule IDs and consistency.
- **Overrides** — The IR supports **overrides** that replace the default behavior for a token or nest. **Simple** overrides (e.g. “use this scope for this token”) can be expressed via configuration (node bindings, token scopes). **Complex** overrides (e.g. line comment with capture, block comment regions, regex literal with special highlighting) require custom logic: the client’s compiler implements an override producer that, given editor context (token, node, etc.), returns a custom **EditorOverride** (custom state rule, extra states, etc.). So: configuration drives the default mapping; the compiler the client implements supplies complex overrides (such as comment overrides) in code, not in the configuration file or .lspec.

### Package `validation`

- LST validation stages and result reporting, implemented **on the Go side**. The `.lspec` grammar has no construct for defining validation rules; validation is semantic and imperative. The DSL compiler runs validation stages after a successful parse; stages and reporting are configurable via **`LSTValidatorConfiguration`**.

### Package `toolchain`

- Helpers for tools (e.g. Sublime config keys and tool names) used by the DSL and generator.

---

## The LangSpec DSL: Syntax Reference

Source files use the extension **`.lspec`**. The structure is:

1. **Header** (required)  
2. **PRAGMA** (optional)  
3. **PATTERN** (optional)  
4. **LEX** (required)  
5. **PRATT** (optional)  
6. **PARSE** (required)

### Header

Exactly one header at the top:

```lspec
--- "DSL name" v1.0.0 | lspec v1.0.0 ---
```

- **`---`** … **`---`** enclose the header.
- Inside: a **string literal** (DSL name), a **version** (`v` + digits and dots), **`|`**, the keyword **`lspec`**, and a **version** again (LangSpec version).
- Example: `--- "MyLang" v1.0.0 | lspec v1.0.0 ---`

### Comments

- **Line comments:** `//` to end of line.
- **Block comments:** defined by the DSL’s block-comment token (matching open/close delimiters).

### PRAGMA section

Optional. Contains tool-specific configuration blocks.

```lspec
PRAGMA {
  tool.sublime {
    enable = true;
    output_path = "path";
    config_path = "path";
  }
}
```

- **PRAGMA** `{` … `}`.
- Inside: blocks of the form **identifier** `{` key-value list `}`.
- Key-value: **identifier** `=` **value** `;` — value is **identifier**, **string literal**, **true**, or **false**.

### PATTERN section

Optional. Named pattern definitions for reuse in LEX (and to keep LEX readable).

```lspec
PATTERN {
  local name : "literal" | `regex`;
  name : other_ref;
  name : "a" "b";
  name : [ 'x'..'z' ];
  name : ! [ 'a' ];
  name : ( A | B ) * ;
  name : ( X ) + ;
  name : ( Y ) ? ;
  name : ( Z ) { 2 , 5 };
}
```

- **PATTERN** `{` … `}`.
- Each rule: optional **local**, then **identifier** `:` **pattern expression** `;`.
- **Pattern expressions:**
  - **String literal** `"..."` — literal text.
  - **Regex literal** `` `...` `` — regex pattern.
  - **Identifier** — reference to another pattern (defined earlier or in the same section).
  - **Concatenation:** space-separated (e.g. `"a" "b"`).
  - **Alternation:** `( A | B )`.
  - **Repetition:** `( X ) *` (0+), `( X ) +` (1+), `( X ) ?` (0 or 1), `( X ) { min , max }` or `( X ) { n , }`.
  - **Character class:** `[ 'a'..'b' ]` (range), `[ 'a' , 'b' ]` (enum), `! [ ... ]` (negated class).
  - **Any character:** `.`
- **local** makes the pattern local (not visible to LEX); without **local**, the pattern name can be used in LEX rules.

### LEX section

Required. Defines token types, roles, and patterns.

```lspec
LEX {
  2 TokenName -> RoleName : pattern_ref;
  1 OtherTok  -> RoleName : `regex`;
  0 EOF       -> structural : eof_pattern;   %% EOF=true %%
}
```

- **LEX** `{` … `}`.
- Each rule: optional **integer** (priority, higher = tried first), **token name** (identifier), **`->`**, **role** (identifier), **`:`**, **pattern** (pattern name or regex literal), **`;`**.
- Optional **meta block** after the pattern: **`%%`** key **`=`** value **`%%`** (e.g. **`%% EOF=true %%`** for the EOF token).
- Pattern is either a **PATTERN** name or a **regex literal** `` `...` ``.

### Meta block (inline)

Used in LEX rules:

```lspec
%% key = value %%
```

- **`%%`** … **`%%`**.
- **key** = identifier, **value** = string, **true**, or **false**.

### PRATT section

Optional. Defines Pratt-style expression grammar (primary, prefix, postfix, infix, implicit). The **prefix**, **postfix**, **infix**, and **implicit** categories are **nested blocks** (each has its own `{` … `}` containing a list of operator definitions), not single statements.

```lspec
PRATT {
  local expr {
    primary {
      ident;
    }
    prefix {
      "-" -> node_name precedence 40;
      "!" -> node_name precedence 41;
    }
    postfix {
      "++" -> node_name precedence 30;
    }
    infix {
      "+" -> node_name precedence 20 21;
      "*" -> node_name precedence 22 23;
    }
    implicit {
      -> node_name precedence 20 19;
    }
  }
}
```

- **PRATT** `{` … `}`.
- Each expression definition: optional **local**, **identifier** (expr name), **`{`** … **`}`** containing:
  - **primary** **`{`** list of rule refs **`;`** **`}`**
  - **prefix** **`{`** … **`}`** — each line: symbol **`->`** node name **precedence** right-binding **integer** **`;`**
  - **postfix** **`{`** … **`}`** — each line: symbol **`->`** node name **precedence** left-binding **integer** **`;`**
  - **infix** **`{`** … **`}`** — each line: symbol **`->`** node name **precedence** left **integer** right **integer** **`;`**
  - **implicit** **`{`** … **`}`** — each line: **`->`** node name **precedence** left right **`;`**

(When emitting .lspec from a grammar, the generator may leave PRATT as a comment indicating it was flattened into PARSE.)

### PARSE section

Required. Defines grammar rules. The grammar distinguishes **rule references** from **token references** by syntax: a bare **identifier** is a rule reference (another parse rule); a **token reference** that is not **virtual** must declare an **output node** in the form **output_node : token_ref** (the LST node kind for the token, then the token name). This is how the parser knows whether an identifier refers to a rule or a token.

```lspec
PARSE {
  IGNORE { whitespace_role; comment_role; };

  rule_name -> NodeKind {
    elem_opt?;
    elem_plus+;
    elem_star*;
    SomeNode : token_name;
    other_rule;
    ( group );
    virtual token_name;
    nest open_tok close_tok { body_rule; };
    sync ( recovery_tok );
    predict ( 0 = first_tok, 1 = second_tok );
  };
}
```

- **PARSE** `{` … `}`.
- Optional **IGNORE** **`{`** list of **role** identifiers **`;`** **`}`** — tokens with these roles are skipped by the parser.
- Each rule: **rule_name** (identifier) **`->`** **node kind** (identifier), optional **transparent**, optional **sync** **`(`** tokens **`)`**, **`{`** body **`}`** **`;`**.
- **Body** is an expression of:
  - **Token reference with output node** — **output_node** **`:`** **token_ref** (token_ref must match a LEX token name). Produces an LST node of kind output_node for that token.
  - **Rule reference** — bare **identifier** (another parse rule).
  - **Group** — **`(`** expression **`)`**.
  - **Alternation** — **expr** **`|`** **expr**.
  - **Optional** — **expr** **`?`**.
  - **One-or-more** — **expr** **`+`**.
  - **Zero-or-more** — **expr** **`*`**.
  - **Repetition** — **expr** **`{`** min **`,`** max **`}`** (or **`{`** n **`,`** **`}`**).
  - **virtual** **token_ref** — token is matched but not kept in the tree (no output node).
  - **nest** **open** **close** **`{`** body **`}`** — nested structure with sync/recovery.
  - **sync** **`(`** token list **`)`** — recovery set for this rule.
  - **predict** **`(`** offset **`=`** token **`,`** … **`)`** — lookahead for disambiguation.

### Literals and identifiers

- **String literal:** **`"`** … **`"`** with escapes (e.g. `\"`, `\n`).
- **Regex literal:** backticks **`` ` ``** … **`` ` ``** (raw regex).
- **Char literal:** **`'`** one character (or escape) **`'`**.
- **Version:** **`v`** followed by digits and dots (e.g. `v1.0.0`).
- **Integer:** digits.
- **Identifier:** letter or `_`, then letters, digits, `_`, `-`.

### Punctuation (summary)

| Symbol | Use |
| ------ | --- |
| `---` | Header delimiters |
| `%%` | Meta section delimiters |
| `\|` | Alternation (patterns, parse) / header separator |
| `->` | LEX token→role, PARSE rule→node, PRATT symbol→node |
| `:` | LEX pattern, PARSE output-node : token-ref |
| `;` | Statement end |
| `{` `}` | Blocks, repetition bounds, character class |
| `(` `)` | Grouping, sync, predict |
| `[` `]` | Character class |
| `,` | List separator, repetition bounds |
| `.` | Any character (pattern) / dot in version |
| `*` `+` `?` | Repetition (0+, 1+, 0-or-1) |
| `!` | Negation (pattern) |
| `=` | Assignment (pragma, meta) |

---

## Use Cases

- **Define a language:** Write a `.lspec` file (header, LEX, PARSE, optional PATTERN/PRAGMA/PRATT), compile with `LangSpecCompilerCompile`, then use `LangSpecCreate` and `LangParserCreate` to parse source files.
- **Bootstrap / self-host:** The LangSpec DSL is itself defined and parsed by this pipeline; the compiler uses the same parser for `.lspec` files.
- **Generate .lspec:** Use `dsl/generator` to emit `.lspec` from an in-memory grammar and lexer (e.g. for round-trip or tooling).
- **Editor support:** Use `langspec/editor` and `editor/sublime` to build Sublime Text syntax from a grammar and lexer; for the DSL, use `dsl/editor.BuildSublimeSyntaxForDSL`.
- **Validation:** Implement validation in Go (package **`validation`**); attach stages to the DSL compiler or run them on the LST after parsing. The `.lspec` file does not define validation rules. Inspect `ValidationEntries` in `LangSpecCompileResult`.

## Safety Guidelines

- **File extension:** `LangSpecCompilerCompile` accepts only paths ending in `.lspec`; otherwise it returns an error.
- **Compiler lifecycle:** Call **`LangSpecCompilerDestroy`** when done with a compiler to release the internal parser and avoid leaks.
- **Parser lifecycle:** Call **`LangParserDestroy`** when done with a parser.
- **Sessions:** Use one session per parse (or reset with **`Reset`**); do not use a session concurrently.
- **Diagnostics:** If no diagnostic sink is set, no trace/LST/validation output is written; result is still returned in **`LangSpecCompileResult`**.
- **Errors:** Even on syntax or validation errors, **`LangSpecCompilerCompile`** may return a non-nil result; check **`SyntaxErrors.HasErrors()`** and validation severities before using **CompiledLexerSpec** / **CompiledParserSpec**.

## Implementation Notes

- The DSL compiler builds a single lexer state (e.g. `"default"`) and a single grammar package; PRATT is compiled into the grammar where supported.
- Token resolution uses longest-match then priority. Delimited tokens (e.g. block comments, strings) use open/close patterns from the lexer spec.
- Validation is not part of the `.lspec` grammar; it is implemented in Go. Validation stages run after a successful parse and can produce entries with different severity levels; the compiler can report them via the configured sink.

## Accuracy and Limitations

- The DSL syntax and semantics are implemented to match the grammar and compiler in this repository. Edge cases in pattern recursion, PRATT, or recovery may behave differently than hand-written parsers; validate with your intended inputs.
- Regex literals are passed to the underlying pattern engine (Regula); not all regex dialects are supported (see `autarch/pattern`).
- Generated Sublime syntax is best-effort; some nested or stateful constructs may require manual overrides in the editor IR.

---

## Bugs and Contributing

LangSpec is under active development. **Bugs and limitations may still exist** in the DSL parser, the compiler, the runtime parser, or the editor generators. If you hit incorrect behavior, unclear errors, or missing features, please consider contributing.

**Contributions are welcome** in the form of:

- **Pull requests** — Fixes, documentation improvements, or small features. Prefer focused PRs with a clear description of the change and how it was tested.
- **Issues** — Bug reports (with minimal `.lspec` or code to reproduce) and feature requests.

When contributing:

- Follow the project’s style guide (`DEVELOPER_README_STYLE_GUIDE.md`) and architecture (data vs. execution, separation of concerns, no methods for core logic).
- Do not add `*_test.go` files for general tests; tests live in `_tests.go` or similar under the project’s own test framework.

Thank you for helping improve LangSpec.
