# LangSpec walkthrough: build, compile a spec, hello world, tests

This guide assumes you are in the **ruleforge** repository root (the directory that contains `go.work`). LangSpec is a normal Go module (`libs/langspec`) wired into that workspace together with `lexarch`, `syntaxa`, `memforge`, and the rest.

---

## 1. What you are building (two different “compiles”)

People often mean one of two things:

1. **Compile the Go code** — build the `langspec` package (and tools) like any other Go library.
2. **Compile a `.lspec` file** — run the LangSpec DSL compiler so a text spec becomes lexer/parser data your program can use. In code this is usually `bootstrap.CompileParserFromSpec` (runtime parser) or `dsl.LangSpecCompilerCompile` (full compile result). Nothing writes generated Go or Sublime files unless you run the **toolchains** (see below).

---

## 2. Prerequisites

- **Go 1.25** (see `go.work` at the repo root).
- A checkout where **`go.work`** lists `./libs/langspec` and its dependencies (as in this repo). If you open only `libs/langspec` in isolation without the workspace, you would need your own `go.work` or `replace` directives pointing at sibling modules — the upstream layout expects the workspace.

---

## 3. Compile the Go code (sanity check)

From the **repository root**:

```bash
go build ./libs/langspec/...
```

You should get no output and exit code 0. That compiles the core runtime, the DSL (`langspec/dsl`), `bootstrap`, `toolchain`, etc.

---

## 4. Hello world: tiny language + parse some text

There is a deliberately small spec at [`examples/hello_world.lspec`](../examples/hello_world.lspec) and a helper command that:

1. Compiles that `.lspec` into a `LangParser` (DSL “compile”).
2. Writes your input to a temp file (the public API parses **files**).
3. Parses and prints whether it succeeded.

From the **repository root**:

```bash
go run ./libs/langspec/cmd/hello-lspec
```

Default input is the word `hello`. Try:

```bash
go run ./libs/langspec/cmd/hello-lspec -text hello
go run ./libs/langspec/cmd/hello-lspec -text hi
```

That stock spec only allows **lowercase letters** as words; other input should fail with a syntax error.

To see **validation and compiler messages** (useful when your own spec breaks):

```bash
go run ./libs/langspec/cmd/hello-lspec -v
```

To point at another spec:

```bash
go run ./libs/langspec/cmd/hello-lspec -spec path/to/your.lspec -text "..."
```

### What to copy into your own project

Typical integration (conceptually):

1. Add a `.lspec` file (header, `PATTERN` / `LEX` / `PARSE`, and a root parse rule named **`PROGRAM`** — see the hello example and [`syntax.md`](syntax.md)).
2. Create a **scratch allocator** (see `memforge` usage in `libs/langspec/dsl/tests/helpers.go` in this repo).
3. Call **`bootstrap.CompileParserFromSpec("your.lspec", alloc, opts...)`** to get a parser.
4. **`defer langspec.LangParserDestroy(parser)`** when done.
5. Create a **`langspec.LangParserSessionCreate[rune](path, nil, false)`** for a real file path, then **`langspec.LangParserParseFile(parser, session)`**.

Codegen (Go bindings, Sublime YAML) is separate: configure `PRAGMA` in the `.lspec`, then run toolchains — for example the **`langspec-toolchain`** command in [`cmd/langspec-toolchain`](../cmd/langspec-toolchain/main.go), or the `bootstrap.RunToolchainsFromSpecFile` API described in the main README.

---

## 5. Running tests (this repo)

LangSpec’s DSL integration checks live under `libs/langspec/dsl/tests/` in files named `*_tests.go`. The **standard Go test runner only picks up `*_test.go`**, so those files are not discovered as separate packages. Instead, the **`tests`** module at the repo root calls **`langspec/dsl/tests.RunAllLangSpecTests`** from its entry test (see `tests/entry_test.go`).

From the **repository root**, the same path the Makefile uses:

```bash
go test -tags=memforge_debug ./tests
```

Or use the full pipeline (runs `go generate` in each module first, then builds and runs the test binary):

```bash
make test
```

If you only want a quick signal that **LangSpec still builds** after your edits, `go build ./libs/langspec/...` from the root is enough; use `./tests` when you need the integrated checks.

---

## 6. Where to read next

- **[`README.md`](../README.md)** — architecture, validation stages, toolchains, packages.
- **[`syntax.md`](syntax.md)** — `.lspec` syntax details.
- **[`examples/lspec.lspec`](../examples/lspec.lspec)** — full LangSpec-in-LangSpec definition (large, but authoritative).
