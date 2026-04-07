# Sublime syntax generation algorithm

This document describes how LangSpec turns a compiled language definition (lexer + grammar) into a Sublime Text **`.sublime-syntax`** file. It is aimed at contributors and advanced users who need to reason about correctness, limitations, and extension points.

For the **`.lspec` surface syntax** (LEX, PARSE, PRAGMA), see **[syntax.md](syntax.md)**.

---

## 1. End-to-end pipeline

At a high level:

```text
GrammarPackage + LexingRuleSet
        │
        ▼
  EditorIRCreate
   ├─ lowering.BuildStateGraph (syntaxa)
   └─ EditorIRFromStateGraph (langspec/editor)
        │
        ▼
     EditorIR  ──►  sublime.GenerateSyntaxFile  ──►  .sublime-syntax (YAML)
```

**Typical entry points**

- **`editor.EditorIRCreate(lexingRuleset, grammarPackage, config)`** — builds **`EditorIR`** when you already have compiled specs.
- **`toolchain.RunSublimeGenerator(SublimeRunnerConfig{...})`** — creates the IR and writes one or more output paths (used by PRAGMA-driven toolchains and helpers such as `RunToolchainsFromSpecFile`).
- **`toolchain.RunTMCommentsToolchain`** — writes `.tmPreferences` for toggle-comment variables; run from the same bootstrap pipeline when `tool.tm_comments` is enabled (see **`docs/syntax.md`** PRAGMA reference).
- **`dsl/editor.BuildSublimeSyntaxForDSL`** — LangSpec-in-LangSpec: typed ruleset + manifest wired into the same IR + generator.

The generator itself lives in **`langspec/editor/sublime`** (`GenerateSyntaxFile`, `syntax_generator.go`). The IR and lowering glue live in **`langspec/editor`** (`ir.go`, `ir_from_state_graph.go`, `ir_lexer_reachability.go`).

---

## 2. Parser side: `StateGraph`

**`syntaxa/lowering.BuildStateGraph`** consumes a **`syntaxa.GrammarPackage`** and produces a **`lowering.StateGraph`**:

- **Contexts** — one per distinct parse situation the grammar analysis needs (roughly: positions in the grammar where specific tokens or structures are expected). Each context has stable string IDs and optional **`ContextMeta`** (optional-continuation fallthrough, immediate push targets for nest wrappers, etc.).
- **Transitions** — labeled by **`lexarch.TokenKind`**, target context IDs, and a **stack operation** for the *parse* machine (`OpMatch`, `OpPush`, `OpPop`, `OpSet`, lookahead sync variants, …).

The state graph is **grammar-only**: it does not know highlighting scopes or regex strings. It is the spine onto which lexer rules are bound.

---

## 3. Lexer side: grouping rules

The editor layer receives a flat **`LexingRuleSet`**: each rule carries its **lexer state name** (e.g. `INITIAL`, `INNER`), pattern, priority, token kind, and optional stack metadata (`push` / `pop` / `set`).

**`editorLexRulesGroupedByState`** sorts rules per state by **priority (desc)** then **token id**, so “which rule wins for token *T* in state *S*” is deterministic and matches the usual lexer conflict resolution story.

For standard imports, this stage remains host-owned: imported parse symbols can impose lexical obligations, but imported lexer rules/states are not injected automatically.  
For `embed` imports, lexer states/rules are merged into the host with namespaced mode keys (`Alias::State`) before editor IR generation, so reachability and emitted `lex__*` contexts can include embedded modes.

---

## 4. Correlating parse contexts with lexer stacks

The parser’s state graph does not carry the lexer’s mode stack. The editor therefore **simulates** lexer stacks alongside parse transitions.

### 4.1 Stack model (aligned with `lexarch`)

The simulation tracks an ordered list of **mode names** (strings), **bottom → top**, with these invariants:

- The stack always contains at least **`INITIAL`** as the bottom logical frame, matching a `lexarch` session that starts with a bottom marker plus the lexer’s start state.
- Embedded handoff rules can push namespaced states (for example `SQL::INITIAL`) onto this stack; the simulation treats them as ordinary lexer mode names.
- **`push(A, B, …)`** appends targets in order (new top is the last target).
- **`pop(k)`** removes up to *k* frames from the top but **never** removes below **`INITIAL`** (same clamping idea as `LexingSessionPop`).
- **`set(targets…)`** is **`pop(1)`** then **`push(targets…)`**, consistent with `LexingSessionSet`.

The depth of this abstract stack is capped at **`editorLexerMaxModesOnStack` (15)**, consistent with `lexarch`’s fixed maximum stack depth including the bottom frame.

### 4.2 Reachability (`editorComputeLexerStackReachability`)

A worklist BFS explores **pairs** `(parseContextID, stackKey)`:

1. Start at **`sg.RootContextID`** with stack **`["INITIAL"]`**.
2. For each dequeued pair `(ctx, stack)`, consider every **grammar transition** from `ctx`.
3. Let **`topMode`** be the top of `stack`. Find the lexing rule for **`(topMode, transition.Token)`**. If there is **no** rule, this edge is **not** followed for this stack (the lexer could not emit that token in that mode).
4. Apply that rule’s **lexer stack operation** to obtain **`nextStack`**.
5. For each **target** parse context on the transition, enqueue **`(target, nextStack)`** if that pair was not seen before.

The result is **`reached[parseContextID] → set of stack keys`** encoding all stacks that can occur when the parser is in that context.

### 4.3 Binding a rule to each grammar transition (`editorResolveLexingRuleForParseTransition`)

When building **`EditorIR`** transitions for parse context **`ctxID`** and grammar token **`tok`**:

1. Take all **reachable stacks** at `ctxID` (if none were discovered, default to the initial **`["INITIAL"]`** stack).
2. **Filter** to stacks whose **top mode** actually defines **`tok`** in the grouped rules.
3. If **no** stack can lex **`tok`**, the transition gets **`lexOK == false`** and is omitted from the default pattern-backed transition path (same as “grammar mentions a token the lexer cannot produce here”).
4. If **two or more** filtered stacks yield **different** emissions (regex string from **`Pattern.ToRegEx()`** plus stack metadata signature), **`EditorIRFromStateGraph` returns an error** — the grammar and lexer are **ambiguous** for highlighting unless parse contexts are refined or rules are unified.
5. Otherwise the unique rule supplies **`OnPattern`** and lexer stack fields copied into **`EditorTransition`** (`LexPushStates`, `LexPopAmount`, `LexSetStates` via **`copyLexStackFromLexingRule`**).

This is the core of **“parser-sparse”** highlighting: each emitted match rule uses the **lexer pattern for the correct mode**, not a single global pattern per token.

### 4.4 Ambient (non-grammar) tokens

Tokens that appear in the lexer but **not** as grammar terminals are collected and emitted as **`LanguageMachine.AmbientTransitions`**, sorted by priority. In Sublime they are written into the **`prototype`** context so they apply globally without a separate transition per parse state.

---

## 5. `EditorIR` shape

**`EditorIR`** combines **`LanguageMeta`** (name, version) and **`LanguageMachine`**:

- **`EditorState`** — one per parse context (plus synthetic states for delimited/foreign overrides). Carries **`Context`** from your **`contextProducer`**, outgoing **`EditorTransition`** list, optional **immediate push** (nest body), **fallthrough pop** metadata, and optional **invalid fallback** (catch-all scope).
- **`EditorTransition`** — regex (from pattern or override), **match context** (scopes), **parse stack op** (`STACK_PUSH` / `POP` / `SET` / `EMBED` / `NONE`), **targets**, **pop amount**, optional **captures**, **foreign/delimited** payloads, and **lexer stack** fields for Sublime `push`/`pop`/`set` on **`lex__*`** contexts.

**`LexerModeStates`** on the machine is intentionally **not** populated for the current Sublime path; lexer modes are represented only where needed via **`lex__<sanitizedState>`** names on transitions.

---

## 6. Sublime YAML emission (`syntax_generator.go`)

### 6.1 Context labels

Lowering labels parse contexts with stable strings. The synthetic **root** state uses **`editor.ROOT_LABEL`** (`"root"`), which the Sublime backend maps to the reserved context name **`main`**.

### 6.2 State minimization (equivalence classes)

Before emitting YAML, **`minimizeStatesForEmission`** partitions **`EditorState`s** that are **indistinguishable** for output:

- Each state gets a **local signature** (meta_scope, immediate push flag, fallthrough / invalid flags, and a canonical encoding of transitions: regex, scopes, captures, parse op, targets).
- The algorithm then refines partitions until **fixed point**: transitions that **push/set/include** other contexts use **partition ids** of targets so two states that only differ by **renamed but equivalent** successors merge.

**`labelToRepresentative`** remaps every original label to its partition representative. All **`push`/`set`/`include`** targets are rewritten to that representative.

### 6.3 Shared `include` extraction

Identical **base transition lists** shared by multiple representatives can be hoisted into a **`shared__*`** context and referenced with **`include`**, when reuse counts justify it (**`shouldExtractSharedInclude`**).

### 6.4 Per-context embellishments

After base entries (or a shared include), the emitter may append:

- **Immediate push** — a synthetic lookahead **`(?=[\s\S]*)`** rule that **`push`**es the nest-body context (Sublime enters the inner context without consuming input).
- **Fallthrough pop** — **`(?=\S)`** with **`pop`** when optional-continuation metadata says the parse machine should pop on non-whitespace.
- **Invalid fallback** — a **`\S`** rule with the “unexpected token” scope when the state has explicit transitions but no fallthrough pop (avoids unscoped text).

### 6.5 Mapping `EditorTransition` → YAML `contextEntry`

For each transition:

- **Match** — `ToRegEx()` on **`OnPattern`**, or an override **`RegexPattern`**; lookahead wraps with **`(?=…)`**.
- **Scope** / **captures** — from **`ExtractionConfig.ExtractScope`** (and meta_scope on embed paths).
- **Parse stack** — **`applyStackOperationRemapped`**: `STACK_PUSH` → YAML **`push`**, `STACK_POP` → **`pop`**, `STACK_SET` → **`set`**, `STACK_EMBED` → **`embed`** / **`escape`** / **`embed_scope`**.
- **Lexer stack** — **`applyLexModeStack`**: lexer **`push`** targets become contexts named **`lex__<sanitizedState>`** (after representative remapping if those names collided with parse partitions). Lexer **`pop`** adds to YAML **`pop`**; lexer **`set`** sets YAML **`set`**.

So a single YAML line can combine **parse** `push`/`pop`/`set` with **lexer** `lex__` operations on the same match.

### 6.6 `prototype` and `main`

- **`main`** holds the minimized **root** parse context’s entries (plus the embellishments above).
- **`prototype`** receives **ambient** transitions when present.

Sublime loads **`main`** by default; **`prototype`** is merged everywhere per Sublime’s rules.

### 6.7 Pruning unreachable contexts

**`pruneUnreachableContexts`** runs a BFS from **`main`** and **`prototype`**, following **`push`**, **`set`**, and **`include`** references. Any non-reserved context not reached is **removed**; if a **`pruneWarnings`** writer is provided (e.g. **`os.Stderr`** in the toolchain), each removed context is logged. This drops dead states after minimization and sharing.

### 6.8 File layout

**`GenerateSyntaxFile`** writes:

1. A generated header (timestamp, language name/version).
2. **YAML metadata** (`name`, `file_extensions`, `scope`, `version: 2`).
3. The **`contexts:`** map via **`yaml.Marshal`**.

---

## 7. Configuration hooks (`EditorIRConfiguration`)

Callers supply:

- **`contextProducer(EditorCtx) TContext`** — maps parse position (token, node kind, nest wrapper, invalid fallback) to your **scope carrier** (e.g. **`toolchain.SublimeContext`**).
- **`overrideProducer`** — optional **per-token** list of **`EditorOverride`** (custom pattern, regex string, delimited regions, foreign embeds).
- **`contextsEqual`** — equality on **`TContext`** (carried on the config for **`EditorIRConfiguration`**; see **`ir.go`**).
- Hashers for tokens and node kinds when building the state graph.

**`ExtractionConfig`** (passed to **`GenerateSyntaxFile`**) turns **`TContext`** into **`scope`** and **`meta_scope`** strings for YAML. Sublime state minimization (**§6.2**) keys off those extracted strings plus transition shape (regex, ops, targets), not on **`TContext`** values directly.

---

## 8. Limitations and failure modes

- **Ambiguous lexer binding** — same parse context and grammar token, but **different** lex rules (or regex/metadata) on **two reachable stacks** that both define that token → **error** from **`EditorIRFromStateGraph`**.
- **Stack overflow in simulation** — exceeding **`editorLexerMaxModesOnStack`** → **error**.
- **Pattern → regex errors** — **`ToRegEx()`** failures surface as generator panics or earlier IR errors depending on path.
- **Highlighting vs parsing** — the generated highlighter approximates **legal** parse+lex paths. It does not replace the real parser for diagnostics or recovery.

---

## 9. Primary source files

| Stage | Package / file |
| --- | --- |
| State graph | `syntaxa/lowering/state_graph.go` (`BuildStateGraph`) |
| Lexer reachability + IR build | `langspec/editor/ir_lexer_reachability.go`, `ir_from_state_graph.go` |
| IR types + `EditorIRCreate` | `langspec/editor/ir.go` |
| Sublime YAML | `langspec/editor/sublime/syntax_generator.go` |
| Toolchain wiring | `langspec/toolchain/sublime_builder.go` |
