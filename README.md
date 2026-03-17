# LangSpec

LangSpec is a language specification and parsing ecosystem. It uses a meta-language (the LangSpec DSL) to describe lexers and grammars, providing the tooling to parse source files, generate editor integrations (e.g., Sublime Text), and output automatic Go bindings for type-safe compiler development. 

Validation logic is intentionally excluded from the `.lspec` grammar. Validation is semantic and requires imperative programming, so it is implemented on the Go side.

## Table of Contents
- [Architecture & Pipeline](#architecture--pipeline)
- [Design Philosophy](#design-philosophy)
- [The LangSpec DSL](#the-langspec-dsl)
- [Validation](#validation)
- [Tooling & Integration](#tooling--integration)
  - [Sublime Text Syntax](#sublime-text-syntax)
  - [Go Bindings](#go-bindings)
- [Packages & API](#packages--api)
- [Contributing](#contributing)

---

## Architecture & Pipeline

Language definitions are purely declarative data. All parsing algorithms and execution mechanics live in the runtime engine. You define your language in a `.lspec` file, and the DSL compiler transforms it into runtime structures (`LexerSpec`, `ParserSpec`, `LangSpec`) that drive the generic engine.

```text
DSL source (.lspec)  →  LangSpecCompilerCompile  →  LangSpecCompileResult
                                                           ↓
                                              CompiledLexerSpec, CompiledParserSpec
                                                           ↓
                                              LangSpecCreate  →  LangParser  →  LST
```

The core pipeline operates as: **raw source → lexing → parsing → Lossless Syntax Tree (LST)**. 

---

## Design Philosophy

- **Data vs. Execution:** Language definitions are treated as data. The engine contains no domain logic.
- **Self-Hosting:** The LangSpec DSL is parsed by its own engine.
- **Separation of Concerns:** Lexer specs, parser specs, and editor IRs are fully independent. 
- **Performance:** NFA-to-DFA lexer compilation with memory bounds, grammar-driven parsing with Pratt expression support, and single-pass LST compilation.

---

## The LangSpec DSL

LangSpec uses a custom meta-language to define both lexing and parsing rules in a single `.lspec` file. 

* **[Read the full Syntax Reference](docs/syntax.md)** for detailed rules on patterns, expressions, and grammar constructs.
* **[View a real-world example](examples/lspec.lspec)** to see how LangSpec defines its own syntax.

### 1. Header (Required)
```lspec
--- "MyLang" v1.0.0 | lspec v1.0.0 ---
```

### 2. PRAGMA (Optional)
Configures tooling like editor integrations or code generation.
```lspec
PRAGMA {
  tool.go_bindings {
    enable = true;
    output-path = "path/to/bindings.go";
    package-name = "mylang";
  }
}
```

### 3. PATTERN (Optional)
Named pattern definitions for reuse in the lexer.
```lspec
PATTERN {
  local digit : [ '0'..'9' ];
  number : ( digit )+;
}
```

### 4. LEX (Required)
Defines token types, roles, and match priorities (higher integer = higher priority).
```lspec
LEX {
  2 TokIdent  -> identifier : `[a-zA-Z_]+`;
  1 TokNumber -> numeric    : number;
  0 EOF       -> structural : eof_pattern;  %% EOF=true %%
}
```

### 5. PRATT (Optional)
Defines Pratt-style expression grammars (primary, prefix, postfix, infix, implicit).
```lspec
PRATT {
  local expr {
    primary { ident; }
    infix {
      "+" -> NodeAdd precedence 20 21;
    }
  }
}
```

### 6. PARSE (Required)
Defines the core grammar rules. Uses `output_node : token_ref` to distinguish tokens from rule references.
```lspec
PARSE {
  IGNORE { whitespace; };

  Statement -> NodeStatement {
    NodeIdent : TokIdent;
    ( expr )?;
  };
}
```

---

## Validation

It is critical to distinguish between validating the **LangSpec DSL (`.lspec` files)** and validating **your target language**.

### 1. Meta-Grammar Validation (Automatic)
Validation for `.lspec` files is semantic, imperative, and runs automatically in Go **after** a successful parse. The compiler builds a typed LST and runs a built-in pipeline of validation stages to ensure your grammar definition is logically sound. Results (`diagnostics`, `warnings`, `errors`, `fatal`) are collected in `ValidationEntries`.

| Stage | Validation Focus |
|-------|------------------|
| **0** | Symbol binding, environment collisions, unresolved references. |
| **1** | Structure and reachability (cyclic references, dead rules). |
| **2** | Lexer semantics (ambiguous matches, shadowed tokens). |
| **3** | Pattern semantics (invalid negation, broken repetition bounds). |
| **4** | Grammar safety (left recursion, unbounded optionals, FIRST-set conflicts). |

### 2. Target Language Validation (Client Implemented)
LangSpec **does not** generate semantic validation for the language you define. The `.lspec` file dictates syntax and lexing rules, nothing more. 

You must implement semantic validation for your target grammar yourself. You have two choices:
* **Use LangSpec's Framework:** Leverage our Go-side validation system by using `LSTValidatorConfigurationCreate` to define your own custom pipeline and `LSTValidatorRun` to execute it against your language's compiled root LST node.
* **Build Your Own:** Export the parsed LST and pass it through your own proprietary validation logic.

---

## Tooling & Integration

### Go Bindings
LangSpec can generate type-safe `Token` and `Node` constants from your spec to replace fragile string literals in your Go code.

1. Add `tool.go_bindings` to your `PRAGMA` block.
2. Compile the spec via `bootstrap.CompileParserFromSpec` (or manually call `toolchain.RunGoBindingsToolchain`).
3. Import the generated package to use strict `Token` and `Node` types in your compiler tooling.

### Sublime Text Syntax

![Theme with automatically generated syntax highlighting](examples/lspec_syntax_highlighting.png)

LangSpec builds a Push-Down Automaton IR to generate `.sublime-syntax` YAML files. This can be driven via JSON or in-memory Go configurations.

**Path 1: Using a JSON Manifest**
Add `tool.sublime` to your `PRAGMA` block, specifying `configuration-path`. Compile via `bootstrap.CompileParserFromSpec`. The toolchain will read the JSON, map node/token bindings to scopes, and generate the YAML.

**Path 2: In-Memory Manifest (Advanced)**
Pass `WithSublimeToolchain(manifest, factory, fileExtensions, scopeExtension)` to the bootstrap. The `configuration-path` in PRAGMA will be ignored, allowing dynamic, Go-driven generation.

*Note: Complex overrides (like region-based block comments) cannot be expressed in JSON. You must provide a Go-based `OverrideProducer` via the `editor` registry.*

---

## Packages & API

The workspace relies on synchronized git submodules (`lexarch`, `syntaxa`, `autarch`, `foundation`). 

* **`langspec` (Root):** Public API for parsers and specs (`LangSpecCreate`, `LangParserCreate`, `LangParserParseFile`).
* **`dsl`:** The DSL compiler (`LangSpecCompilerCompile`).
* **`validation`:** Go-side LST validation framework.
* **`editor` / `editor/sublime`:** Generic push-down automaton IR and YAML generator for syntax highlighting.
* **`toolchain`:** Helpers for executing Go bindings and Sublime integrations.
* **`bootstrap`:** High-level wrappers (`CompileParserFromSpec`) to quickly go from a `.lspec` file to a ready-to-use parser.

---

## Contributing

LangSpec is under active development. If you encounter edge cases in pattern recursion, Pratt parsing, or recovery, contributions are welcome.

* **Pull Requests:** Keep them focused. Include a clear description and testing methodology. Do not use `*_test.go` files for general tests; follow the project's custom test framework conventions (`_tests.go`).
* **Issues:** Provide a minimal `.lspec` reproduction for bug reports.