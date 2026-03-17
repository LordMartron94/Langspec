# LangSpec

LangSpec is a language specification and parsing ecosystem. It provides a meta-language (the LangSpec DSL) for describing lexers and grammars, plus tooling to parse source files, to generate editor integrations (e.g. Sublime Text syntax definitions), and to generate **automatic Go bindings** (Token and Node types and constants) for type-safe use in downstream compilers and tooling. The `.lspec` grammar itself does **not** include logic for specifying validation—validation is semantic and would require imperative programming, so it is implemented on the Go side (see package **`validation`** and compiler validation stages).

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

### Ergonomics: bootstrap, editor registry, and default patterns

Three conveniences reduce boilerplate when going from a `.lspec` file to a parser and editor support:

- **Bootstrap** — Package **`bootstrap`** provides **`CompileParserFromSpec(specFile, alloc, opts...)`**. It compiles the DSL, runs toolchains (e.g. Sublime syntax generation when the spec enables it in PRAGMA), and returns a **`LangParser`**. Options: **`WithDiagnosticSink(sink)`** for human-readable diagnostics; **`WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension)`** to supply an **in-memory** manifest and override factory (then **configuration-path** in PRAGMA is ignored). If **`WithSublimeToolchain`** is not used, the bootstrap uses **`toolchain.RunSublimeToolchain`**, which reads the manifest from the JSON file at **configuration-path**. Use this when you want “one call” from spec path to parser (and optional editor assets).

- **Editor registry** — Package **`editor`** provides **`OverrideRegistry`** and **`NewOverrideRegistry`**. Register token → **`OverrideHandler`** with **`Register(token, handler)`**; then pass **`Producer()`** as the `overrideProducer` in **`EditorIRConfigurationCreate`** (or use it inside a **`SublimeOverrideFactory`** passed to **`WithSublimeToolchain`**, after adapting to the string-typed toolchain if your spec is compiled dynamically). The registry maps tokens to overrides so you can assemble one producer from many handlers instead of writing a single large switch.

- **Default patterns** — **`TextPatternBuilder`** ( **`NewTextPatternBuilder(factory)`** ) builds **`OverrideHandler`** values for common cases: **`LineComment(prefix, matchContext, punctuationContext)`** and **`BlockComment(open, close, bodyMetaContext, openContext, closeContext)`**. Use these to register line- and block-comment overrides (e.g. for capture or delimited regions) without implementing handlers by hand. Combine with **`OverrideRegistry`**: create a registry, create a **`TextPatternBuilder`** with a rune **`RegulaASTFactory`**, register the handlers for your comment tokens, then use **`Producer()`** as the override producer.

Typical flow: **`CompileParserFromSpec(specFile, alloc, WithDiagnosticSink(sink))`** for JSON-driven Sublime (PRAGMA sets **configuration-path**); or **`CompileParserFromSpec(specFile, alloc, WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension))`** when the manifest is built in Go. For the DSL itself, **`dsl/editor.BuildSublimeSyntaxForDSL`** uses a built-in override producer; for user-defined languages, the manifest can come from a **JSON file** ( **`RunSublimeToolchain`** ) or **in-memory** ( **`WithSublimeToolchain`** + **`RunSublimeToolchainFromMemory`** ).

### Required libraries and workspace

The required libraries (`lexarch`, `syntaxa`, `autarch`, `foundation`, etc.) are meant to be imported into the **same Go workspace** as LangSpec. They are maintained as **git submodules** so the workspace can track a consistent set of versions. These libraries are intended to be available on the maintainer’s GitHub. If you cannot access a dependency (e.g. the repository is private or missing), please **open an issue** on this repository so the maintainer can make the library public or fix the reference.

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

Sublime Text syntax is generated from the editor IR, which is built from a **GrammarPackage** and **LexingRuleset** plus **configuration**. For a step-by-step guide and both entry points (DSL vs user language), see **Generating a Sublime Text syntax file** below.

- **Scope provider / context producer** — Maps tokens and grammar nodes to Sublime scope strings (e.g. `comment.line`, `string.quoted.double`). Configuration supplies a base scope per token and optional per-node bindings (node kind → scopes, and optionally token-specific scopes within a node).
- **Token formatter** — Used for rule IDs and consistency.
- **Overrides** — The IR supports **overrides** that replace the default behavior for a token or nest. **Simple** overrides (e.g. “use this scope for this token”) can be expressed via configuration (node bindings, token scopes). **Complex** overrides (e.g. line comment with capture, block comment regions, regex literal with special highlighting) require custom logic: the client’s compiler implements an override producer that, given editor context (token, node, etc.), returns a custom **EditorOverride** (custom state rule, extra states, etc.). So: configuration drives the default mapping; the compiler the client implements supplies complex overrides (such as comment overrides) in code, not in the configuration file or .lspec.

### Package `validation`

- LST validation stages and result reporting, implemented **on the Go side**. The `.lspec` grammar has no construct for defining validation rules; validation is semantic and imperative. The DSL compiler runs validation stages after a successful parse; stages and reporting are configurable via **`LSTValidatorConfiguration`**.

### Package `toolchain`

- Helpers for tools used by the DSL and generator. **Sublime toolchain:** supports two manifest sources — (1) **JSON manifest**: **`RunSublimeToolchain`** reads PRAGMA **configuration-path**, loads **`SublimeConfiguration`** via **`LoadSublimeConfigFromJSON`**, and runs the pipeline; (2) **in-memory manifest**: **`RunSublimeToolchainFromMemory`** takes a **`SemanticManifest`** and metadata directly, used by the bootstrap when **`WithSublimeToolchain`** is set. Both use **`BuildContextProducerFromManifest`** for the editor IR. **Go bindings toolchain:** **`RunGoBindingsToolchain`** generates a Go source file with **Token** and **Node** types (string-based) and one const per token and per grammar node when **tool.go_bindings** is enabled in PRAGMA; use the generated package for type-safe token and node references in downstream compilers.

---

## Generating a Sublime Text syntax file

This section describes how to use LangSpec to produce a **Sublime Text syntax definition** (`.sublime-syntax` YAML) for a language. The pipeline is: **grammar + lexer + configuration → editor IR → Sublime YAML**. Two main entry points exist: one for the **LangSpec DSL itself** (`.lspec` files), and one for **any language** you define in a `.lspec` and optionally drive with a JSON config and override producer.

### Pipeline overview

1. **Grammar and lexer** — From your `.lspec` (compiled via `LangSpecCompilerCompile` or `CompileParserFromSpec`), LangSpec produces a `GrammarPackage` and a `LexingRuleset`.
2. **Editor IR** — Package **`editor`** builds an **editor IR** (push-down automaton) from that grammar and lexer plus an **`EditorIRConfiguration`**: token hasher, **context producer** (maps tokens/nodes to scope strings), **override producer** (optional custom behavior per token), and context equality.
3. **Sublime YAML** — Package **`editor/sublime`** turns the IR into a `.sublime-syntax` file (contexts, rules, file extensions, scope) via **`GenerateSyntaxFile`**. The **toolchain** package wires steps 2 and 3 for you when using PRAGMA or the bootstrap.

### Path 1: Syntax for the LangSpec DSL (`.lspec` files)

When you want Sublime highlighting **for the DSL itself** (e.g. editing `.lspec` files in your editor):

1. Create and configure a **LangSpec compiler** (e.g. `LangSpecCompilerCreate` with `LangSpecCompilerConfigurationCreate` and optional `WithDiagnosticSink`).
2. Call **`dsl/editor.BuildSublimeSyntaxForDSL(compiler, syntaxFile)`** where `syntaxFile` is the output path for the `.sublime-syntax` file.

No PRAGMA or JSON config is required. The DSL editor package uses the compiler’s **scope map** and a **built-in override producer** (line comment, block comment, regex literal) and writes the file. Use this when you are building tooling for the meta-language.

### Path 2: Syntax for a user-defined language (your `.lspec`)

When your **language** is defined in its own `.lspec` file and you want to generate a Sublime syntax from it, use the **Sublime toolchain**. The manifest can be supplied in **two ways**: (1) **JSON manifest** — a file at **configuration-path** in PRAGMA (see Step 1 and Step 3); (2) **in-memory manifest** — pass **`WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension)`** to the bootstrap so the manifest and metadata come from Go ( **configuration-path** is then ignored).

#### Step 1: Enable the Sublime tool in your `.lspec` PRAGMA

In your `.lspec` file, add a PRAGMA block:

```lspec
PRAGMA {
  tool.sublime {
    enable = true;
    output-path = "path/to/your.sublime-syntax";
    configuration-path = "path/to/sublime_config.json";
  }
}
```

- **`enable`** — Must be `true` for the toolchain to run.
- **`output-path`** — Where the generated `.sublime-syntax` file is written. Required in both JSON and in-memory modes.
- **`configuration-path`** — Path to a **JSON file** that describes file extensions, scope naming, and the **scope manifest**. **Only used when the manifest is not supplied in-memory** (i.e. when you do not use **`WithSublimeToolchain`** ). If you use **`WithSublimeToolchain`**, this setting is ignored and the manifest comes from Go. See **Configuration JSON format** below for the file shape.

The toolchain runs when you compile the spec and either: invoke **`toolchain.RunSublimeToolchain(compileResult, overrideProducer)`** (JSON manifest), use **`toolchain.RunSublimeToolchainFromMemory(...)`** (in-memory), or use the bootstrap (which chooses JSON vs in-memory based on whether **`WithSublimeToolchain`** was passed). It only runs if `enable = true` and **output-path** is set; **configuration-path** is required only for the JSON path.

#### Step 2: Compile the spec and run the toolchain

**Option A — Bootstrap with JSON manifest:** Use **`bootstrap.CompileParserFromSpec(specFile, alloc, WithDiagnosticSink(sink), ...)`** without **`WithSublimeToolchain`**. The bootstrap calls **`toolchain.RunSublimeToolchain(compileResult, nil)`**, which reads **configuration-path** from PRAGMA, loads the JSON config, and writes the syntax file. Override producer is nil unless you call the toolchain separately with one.

**Option B — Bootstrap with in-memory manifest:** Use **`bootstrap.CompileParserFromSpec(specFile, alloc, WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension), ...)`**. The bootstrap calls **`toolchain.RunSublimeToolchainFromMemory`** with your manifest and metadata; **configuration-path** is ignored. The **factory** is called to build the override producer (can be nil if you do not need overrides).

**Option C — Manual:** Compile with **`LangSpecCompilerCompile(compiler, specFile)`** to get a **`LangSpecCompileResult`**. Then call **`toolchain.RunSublimeToolchain(compileResult, overrideProducer)`** (JSON) or **`toolchain.RunSublimeToolchainFromMemory(compileResult, manifest, overrideProducer, outputPath, fileExtensions, scopeExtension)`** (in-memory). The chosen function builds the IR and writes the syntax file.

In both cases, **`overrideProducer`** can be **`nil`** if you do not need token overrides (e.g. no special line/block comment or embedded-region handling). Otherwise, pass a function with signature:

```go
func(ec *editor.EditorCtx[rune, string, string, string, string]) (*editor.EditorOverride[rune, string, string, string, string, toolchain.SublimeContext], bool)
```

The toolchain uses **string** token and node kinds (from the compiled spec); if your override logic is written in terms of **typed** tokens (e.g. `dsl.LangSpecLexerTokenType`), build an **adapter** that maps string token names to your enums and delegates to your typed producer (see **Token overrides** below).

#### Step 3: Configuration JSON format

The file at **`configuration-path`** must be valid JSON matching **`editor/sublime.SublimeConfiguration`**:

| Field | JSON key | Description |
| ----- | -------- | ----------- |
| File extensions | `file_extensions` | List of extensions for this syntax (e.g. `[".mylang"]`). |
| Scope extension | `scope_extension` | Suffix for scope names (e.g. `".mylang"`). The base scope is `source` + this value. |
| Scope manifest | `scope_manifest` | Object with `invalid_scope`, `base_token_scopes`, and `node_bindings`. |

**Scope manifest** (`scope_manifest`):

| Field | JSON key | Description |
| ----- | -------- | ----------- |
| Invalid scope | `invalid_scope` | Scope applied to invalid/unexpected tokens (e.g. `"invalid.illegal.unexpected-token"`). |
| Base token scopes | `base_token_scopes` | Map from **token name** (string, as in your LEX) to default scope (e.g. `"TokIdentifier"` → `"variable.other"`). |
| Node bindings | `node_bindings` | Map from **grammar node kind** (string) to a **binding** object. |

Each **binding** in `node_bindings` can have:

- **`scopes`** — List of scope strings for that node (first is used when no token override).
- **`token_scopes`** — Optional map from **token name** to list of scopes; used when the node is realized by that token (e.g. string literal inside a meta value).

Example minimal config:

```json
{
  "file_extensions": [".mylang"],
  "scope_extension": ".mylang",
  "scope_manifest": {
    "invalid_scope": "invalid.illegal.unexpected-token",
    "base_token_scopes": {
      "TokIdentifier": "variable.other",
      "TokStringLiteral": "string.quoted.double",
      "TokLineComment": "comment.line",
      "TokBlockComment": "comment.block"
    },
    "node_bindings": {
      "NodeKeyword": { "scopes": ["keyword.control"] },
      "NodeStringLiteral": {
        "scopes": ["meta.value"],
        "token_scopes": {
          "TokStringLiteral": ["meta.value", "string.quoted.double"]
        }
      }
    }
  }
}
```

The context producer built by the toolchain uses this manifest to map (token, node) → scope when building the editor IR. Simple “one scope per token/node” behavior is entirely data-driven; **complex** behavior (line comment with prefix capture, block comment region, embedded regex) requires an **override producer** in code.

#### Step 4: Token overrides (optional)

When the default “single transition per token” is not enough (e.g. line comment with punctuation capture, block comment as a delimited region, regex literal as embedded scope), supply an **override producer** to **`RunSublimeToolchain`** / **`RunSublimeToolchainFromMemory`**, or supply a **SublimeOverrideFactory** to **`WithSublimeToolchain`** that returns that producer.

- **OverrideRegistry** — In **`editor`**, create an **`OverrideRegistry`**, register **`OverrideHandler`**s per token (e.g. from **`TextPatternBuilder.LineComment`** / **`BlockComment`**), and use **`Producer()`** as the override producer. The registry’s producer is typed by your token type; the **Sublime toolchain** expects a **string**-typed producer (token names from the compiled spec). So for a **user-defined** language you have two options: (1) implement a string-based producer that maps token name strings to the desired **`EditorOverride`** (e.g. with a switch or map), or (2) keep a typed registry keyed by your enum and an **adapter** that, given `EditorCtx` with string token, looks up the enum and calls your typed registry’s producer.
- **TextPatternBuilder** — Use **`NewTextPatternBuilder(factory)`** (with a rune **`RegulaASTFactory`**), then **`LineComment(prefix, matchContext, punctuationContext)`** and **`BlockComment(open, close, bodyMetaContext, openContext, closeContext)`** to get handlers. Register those for the corresponding tokens. This gives consistent, editor-friendly comment highlighting without hand-written patterns.

Example (conceptual): register line and block comment handlers for your language’s token names, then pass an adapter that maps `ctx.Token` (string) to your enum and calls the registry; the adapter returns the resulting **`EditorOverride`** with **`SublimeContext`** (Scope / MetaScope) filled from your bindings.

### Summary

| Goal | Entry point |
| ---- | ----------- |
| Sublime syntax for **`.lspec`** (the DSL) | **`dsl/editor.BuildSublimeSyntaxForDSL(compiler, syntaxFile)`** |
| Sublime syntax for **your language** with **JSON manifest** | Enable **PRAGMA** `tool.sublime` with **output-path** and **configuration-path**; compile then **`toolchain.RunSublimeToolchain(compileResult, overrideProducer)`** or **`bootstrap.CompileParserFromSpec(...)`** (no **WithSublimeToolchain**). |
| Sublime syntax for **your language** with **in-memory manifest** | Enable **PRAGMA** `tool.sublime` with **output-path**; call **`bootstrap.CompileParserFromSpec(..., WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension))`** or **`toolchain.RunSublimeToolchainFromMemory(...)`** manually. **configuration-path** is ignored. |
| No overrides | Pass **`nil`** as override producer (or nil **SublimeOverrideFactory** when using **WithSublimeToolchain**). |
| Line/block comment or custom token behavior | Implement an override producer (e.g. **`OverrideRegistry`** + **`TextPatternBuilder`**), or a **`SublimeOverrideFactory`** that returns one; optionally adapt typed → string for the string-typed toolchain. |

---

## Generating Go bindings for tokens and nodes

LangSpec can **automatically generate** a Go source file that defines **Token** and **Node** as named types plus one const per lexical token and per grammar node from your compiled spec. This gives you type-safe references to tokens and nodes in downstream code (e.g. validation, codegen, or tooling) instead of magic strings.

### How it works

1. **Enable the toolchain in PRAGMA** — In your `.lspec` file, add a **tool.go_bindings** block with **enable** = true, **output-path** (path to the generated `.go` file), and **package-name** (Go package name for the generated file).

2. **Compile the spec** — When you compile via **`bootstrap.CompileParserFromSpec`** or manually call **`toolchain.RunGoBindingsToolchain(compileResult)`** after **`LangSpecCompilerCompile`**, the toolchain runs if **tool.go_bindings** is enabled. It collects all tokens from the lexer ruleset and all node kinds from the grammar, then writes a single `.go` file.

3. **Generated file** — The file contains:
   - **`type Token string`** and a **`const`** block with one constant per token (e.g. `TokIdentifier`, `TokStringLiteral`).
   - **`type Node string`** and a **`const`** block with one constant per grammar node (e.g. `NodeStatement`, `NodeExpression`).

4. **Use the types** — Import the generated package in your compiler or tooling and use **Token** / **Node** for switch cases, map keys, and function parameters so the type system catches typos and invalid values.

### PRAGMA example

```lspec
PRAGMA {
  tool.go_bindings {
    enable = true;
    output-path = "path/to/bindings.go";
    package-name = "mylang";
  }
}
```

The bootstrap runs **`RunGoBindingsToolchain`** automatically when this pragma is present and enabled (before the Sublime toolchain). When **emitting** an `.lspec` from a grammar via the **generator** package, use **`WithGoBindingsConfiguration(outputPath, packageName)`** on the generator config so the emitted PRAGMA includes **tool.go_bindings** with the given path and package name.

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

- **tool.sublime** — Sublime syntax generation: **enable**, **output-path**, and optionally **configuration-path** (JSON manifest). See **Generating a Sublime Text syntax file**.
- **tool.go_bindings** — Go bindings generation: **enable** = true, **output-path** = path to generated `.go` file, **package-name** = Go package name. Generates **Token** and **Node** types and consts. See **Generating Go bindings for tokens and nodes**.

```lspec
PRAGMA {
  tool.sublime {
    enable = true;
    output-path = "path";
    configuration-path = "path";
  }
  tool.go_bindings {
    enable = true;
    output-path = "path/to/bindings.go";
    package-name = "mylang";
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
- **Editor support:** Use `langspec/editor` and `editor/sublime` to build Sublime Text syntax from a grammar and lexer; for the DSL, use `dsl/editor.BuildSublimeSyntaxForDSL`. For token overrides (e.g. line/block comments), use the **editor registry** and **default patterns** (see **Ergonomics** above).
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

- Do not add `*_test.go` files for general tests; tests live in `_tests.go` or similar under the project’s own test framework.

Thank you for helping improve LangSpec.
