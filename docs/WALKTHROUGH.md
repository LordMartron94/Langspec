# LangSpec Integration Walkthrough: build, compile a spec, hello world

This guide explains how to integrate LangSpec into your own Go project. LangSpec relies on a Go workspace (`go.work`) environment to resolve its sibling dependencies (`lexarch`, `syntaxa`, `memforge`). 

---

## 1. Project Setup (The Workspace)

Because LangSpec and its related packages are designed as interdependent modules, you cannot simply run a standard `go get` to pull a single library. You must pull the required modules into your project tree (often as git submodules or into a `vendor/` or `third_party/` directory) and link them using a `go.work` file at your repository root.

**Example Setup:**
Assuming you place the LangSpec ecosystem into a directory named `submodules/`, your project structure should look like this:

```text
your-project/
├── go.mod             <-- Your project's module file
├── go.work            <-- The workspace file you must create
└── submodules/
    ├── langspec/      <-- The cloned LangSpec repository
    ├── lexarch/       
    ├── syntaxa/       
    └── memforge/      
```

**Your `go.work` file must look like this:**

```go
go 1.25

use (
    . // Your own project's code
    ./submodules/langspec
    ./submodules/lexarch
    ./submodules/syntaxa
    ./submodules/memforge
)
```

Without this workspace configuration, the Go compiler will fail to resolve local paths between LangSpec and its required dependencies.

---

## 2. What you are building (two different “compiles”)

When working with LangSpec, "compiling" usually means one of two things:

1. **Compile the Go code** — Build the `langspec` packages and tools into your project like any other Go library.
2. **Compile a `.lspec` file** — Run the LangSpec DSL compiler to convert a text specification into lexer/parser data that your program can execute. In code, this is handled by `bootstrap.CompileParserFromSpec` (for a runtime parser). Note that this process does not generate Go code or Sublime files automatically unless you explicitly run the **toolchains**.

If you only need generated artifacts (Sublime syntax, Go bindings, TM comments, and similar) and do not plan to embed a runtime `LangParser` in your application, you can skip `hello-lspec` and the parse-session steps below: enable the relevant `PRAGMA` tools in your `.lspec`, then run `go run ./cmd/langspec-toolchain -spec your.lspec` from the LangSpec module (optionally with `-toolchains` to filter steps, and `-sublime-json-config` when Sublime is enabled without `configuration-path` in PRAGMA), or use the `scripts/run_toolchains.sh` helper described in the LangSpec **README** under *Toolchain-only workflow* (Go-config mode delegates to `langspec/cliutil` for in-memory Sublime runs, including Lingua-style configs).

---

## 3. Compile the Go code (sanity check)

To ensure your workspace is resolving the local dependencies correctly, run a build targeting the LangSpec module from your **repository root**:

```bash
go build ./submodules/langspec/...
```

You should get no output and an exit code of 0. This confirms that the core runtime, the DSL, and the toolchains compile successfully within your workspace.

---

## 4. Hello world: tiny language + parse some text

LangSpec includes a deliberately small specification and a helper command to test that your setup can parse text. The helper command compiles a sample `.lspec` file into a `LangParser`, writes input to a temporary file, and parses it.

Run this from your **repository root**:

```bash
go run ./submodules/langspec/cmd/hello-lspec
```

The default input is the word `hello`. Try passing different text:

```bash
go run ./submodules/langspec/cmd/hello-lspec -text hi
```

This specific sample spec (`hello_world.lspec`) only allows lowercase letters as valid words. If you input numbers or uppercase letters, it will fail with a syntax error.

To see **validation and compiler messages** (which is critical when debugging your own `.lspec` files later):

```bash
go run ./submodules/langspec/cmd/hello-lspec -v
```

---

## 5. Integrating LangSpec into your Go code

When you are ready to use LangSpec in your own application, the typical flow looks like this:

1. Add a `.lspec` file to your project. It requires a header, `PATTERN` / `LEX` / `PARSE` sections, and a root parse rule strictly named **`PROGRAM`** (refer to `syntax.md` in the LangSpec docs).
2. Create a **scratch allocator** (using the `memforge` module in your workspace).
3. Call **`bootstrap.CompileParserFromSpec("your.lspec", alloc, opts...)`** to generate the parser.
4. Ensure you clean up memory by calling **`defer langspec.LangParserDestroy(parser)`**.
5. Create a session for a real file path using **`langspec.LangParserSessionCreate[rune](path, nil, false)`**, and execute the parse with **`langspec.LangParserParseFile(parser, session)`**.

*(Note: If you require codegen—like generating Go bindings—you configure the `PRAGMA` in your `.lspec` file and run the toolchain via `bootstrap.RunToolchainsFromSpecFile` or the CLI tool provided in the LangSpec repo).*

---

## 6. Where to read next

Refer to the documentation inside the `langspec` submodule you cloned for deeper architectural details:
- **`README.md`** — Core architecture, validation stages, toolchains, and package structure.
- **`syntax.md`** — Detailed breakdown of `.lspec` syntax.
- **`examples/lspec.lspec`** — The full LangSpec-in-LangSpec definition (a large, authoritative reference).