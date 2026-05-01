# LangSpec DSL Syntax Reference

The LangSpec Domain Specific Language (DSL) defines the complete lexical and syntactic structure of a target language in a single `.lspec` file. 

For the foreseeable future, LangSpec supports the LangSpec DSL (`.lspec`) as the authoring format. Alternative meta-grammars (such as EBNF) are not planned as first-class compiler inputs because translating them into `.lspec` is complex and generally provides little benefit while reducing access to LangSpec-specific features and semantic validation guarantees.

If you want to use LangSpec, write your grammar in the LangSpec DSL. Adapter efforts for other meta-grammars are still welcome as external tooling or pull requests.

A `.lspec` file evaluates sequentially and expects up to six distinct sections. Only the **Header**, **LEX**, and **PARSE** sections are strictly required.

## 1. Document Structure

The required order of sections is as follows:

1. **Header** (Required)
2. **PRAGMA** (Optional)
3. **PATTERN** (Optional)
4. **LEX** (Required)
5. **PRATT** (Optional)
6. **PARSE** (Required)

### Comments and Literals
* **Line Comments:** `// comment` (Extends to the end of the line).
* **Block Comments:** `/* comment */` (supports multiline)
* **String Literals:** `"text"` (Supports standard escapes like `\n`, `\t`, `\"`).
* **Regex Literals:** `` `[a-z]+` `` (Raw regular expressions evaluated by the underlying engine).
* **Char Literals:** `'a'` (Single character).

---

## 2. Header

The header declares the target language name and version, alongside the required LangSpec compiler version. It must be the first non-comment line in the file.

**Syntax:**
`--- "<LanguageName>" v<LanguageVersion> | lspec v<CompilerVersion> ---`

**Example:**

```
--- "MyLang" v1.0.0 | lspec v1.0.0 ---
```

---

## 2.5 IMPORT and module exports

LangSpec supports composing grammars from other `.lspec` modules through an explicit import system.

### IMPORT block

Import declarations live in an optional top-level `IMPORT` block:

```
IMPORT {
  "path/to/module.lspec" as M;
  embed "path/to/sql.lspec" as SQL;
  "/absolute/path/other.lspec" as Other;
}
```

Rules:

* Each entry uses a quoted file path and an alias: `"..." as Alias;`
* Prefix an import with `embed` to mark it as an embedded-language module: `embed "..." as Alias;`
* Alias names must be unique inside the file.
* Imported symbols are referenced through the alias namespace (`M.<Name>`).

### Embedded module imports

`embed` imports are different from standard structural imports:

* Their lexer artifacts are merged into the host with a namespaced prefix:
  * states: `Alias::State` (for example `SQL::INITIAL`)
  * tokens/roles: `Alias::Name`
* They are intended for `embed Alias nest OpenTok CloseTok` parse statements.
* `embed` parse statements must reference an alias declared with `embed`; using a standard import alias is invalid (`V_IMP009`).

### Exporting symbols from a module

In `PARSE`, declarations can be exported so other modules can reference them:

```
PARSE {
  export pair Braces TokOpen TokClose;
  export rule Item -> ItemNode { virtual TokWord };
  export template ItemTpl($r : Rule) { $r };
}
```

This enables external use through `using <Alias>.<ExportedName>` and `using <Alias>.<ExportedTemplate>(...)`.

Patterns can also be exported from `PATTERN` and referenced from importing modules (for example from lex `using` sites). This is part of the same import/export symbol surface.

### Referencing imported symbols (`using`)

Imported parse-side symbols are used via `using` (not via inline dotted parse references):

```
PARSE {
  rule PROGRAM -> ProgramNode {
    nest using M.Braces {
      using M.Item
      using M.ItemTpl(PROGRAM)
    }
  };
}
```

Important constraints:

* Template usage must include call arguments:
  * valid: `using M.TemplateName(arg1, arg2)`
  * invalid: `using M.TemplateName`
* Non-template exports must not be called with arguments:
  * valid: `using M.RuleName`
  * invalid: `using M.RuleName(...)`
* Unknown exports are rejected during semantic validation.
* Current import surface is intentionally limited to exported parse-side constructs used via `using`; direct external token or Pratt import usage is not part of the current public syntax surface.
* In the `LEX` section, `using Alias.Symbol` is validated as a pattern-site reference: the symbol must resolve to an exported pattern. Wrong-category or missing exports are reported (`V_IMP006` / `V_IMP005`).

Validation codes related to import/export misuse include:

* `V_IMP005`: unresolved import/export symbol (alias or exported name not found).
* `V_IMP006`: invalid import `using` category/shape (for example, missing template call syntax or calling a non-template as a template).

### Imported lexical obligations

Imported parse symbols are treated as **lexical contracts**:

* `using Alias.Rule`, `using Alias.Template(...)`, and `nest using Alias.Pair` can require host tokens used by that exported symbol.
* The host must declare required tokens in its own `LEX` section; imported token rules are not auto-materialized.
* If an imported symbol implies stack transitions, the host must also declare required lexer states.
* Missing requirements fail validation with explicit import diagnostics:
  * `V_IMP007`: missing host token obligation.
  * `V_IMP008`: missing host lexer-state obligation.
* `using Alias.Pattern` in `LEX` is still supported as pattern reuse, but priority/role/state behavior remains host-owned.

### Library module pragma (`lspec`)

Library modules can opt out of executable-entrypoint checks by declaring:

```
PRAGMA {
  lspec {
    library = true;
  }
}
```

Behavior:

* Imported modules are parsed and validated through the same stage-0..3 pipeline used by root specs.
* If an imported module fails syntax or validation, root compilation fails and includes import-context diagnostics (`import '<alias>' (<path>): <code> <message>`).
* In `library = true` modules, executable-only checks are relaxed:
  * `V_PAR004` (`PROGRAM` required) is disabled.
  * unresolved-local-reference checks (`V_PAR002`, `V_PAT002`, `V_LEX006`) are relaxed for that library module.
* Import/export contract checks (for example `V_IMP005`, `V_IMP006`) remain active in library mode.

---

## 3. PRAGMA

The `PRAGMA` section defines tooling and compiler directives.

**Syntax:**

    PRAGMA {
      tool.<tool_name> {
        key = value;
      }
      lspec.library {
        library = true;
      }
    }

**Supported Values:** Strings, `true`, `false`, or bare identifiers.

**Example:**

```
PRAGMA {
  tool.sublime {
    enable = true;
    output-path = "syntax.sublime-syntax";
  }
  tool.go_bindings {
    enable = true;
    package-name = "parser";
  }
  tool.tm_comments {
    enable = true;
    output-path = "Comments.tmPreferences";
    scope-extension = ".mylang";
    single-line-comment-start = "// ";
    block-comment-start = "/*";
    block-comment-end = "*/";
  }
}
```

### tool.tm_comments

Drives generation of a Sublime Text **`.tmPreferences`** file (toggle-comment shell variables) when toolchains run (`RunToolchainsFromSpecFile`, `langspec-toolchain`, etc.).

| Key | Required | Description |
|-----|----------|-------------|
| `enable` | yes | `true` or `false` |
| `output-path` | yes | One path string or array of paths (same as other tools) |
| `scope` | one of `scope` / `scope-extension` | Full scope, e.g. `source.mylang` |
| `scope-extension` | | Suffix only; combined as `source` + value (matches Sublime syntax base scope) |
| `single-line-comment-start` | yes | Prefix for line comments (often includes a trailing space) |
| `block-comment-start` / `block-comment-end` | both or neither | If set, block toggle comments are included in the plist |

---

## 4. PATTERN

The `PATTERN` section defines reusable lexical building blocks to keep the `LEX` section clean. Patterns are purely textual substitutions and do not produce tokens themselves.

**Expressions:**
* **String Literal:** `"if"`
* **Regex Literal:** `` `[0-9]+` ``
* **Character Class:** `[ 'a'..'z', 'A'..'Z', '_' ]`
* **Any Character:** `.`
* **Alternation:** `( expr1 | expr2 )`
* **Sequence:** `expr1 expr2`

**Prefix Operators:**
* **Negation:** `!` can be applied to *any* pattern expression (e.g., `!['\n']` or `!"forbidden"`).

**Repetition Modifiers:**
* `*` : Zero or more.
* `+` : One or more.
* `?` : Zero or one.
* `{min, max}` : Exact bounds. Also supports `{min,}` (no max) and `{,max}` (no min).

**Example:**

```
PATTERN {
  local digit : [ '0'..'9' ];
  local alpha : [ 'a'..'z', 'A'..'Z', '_' ];
  
  identifier : alpha ( alpha | digit )*;
  whitespace : [ ' ', '\t', '\n', '\r' ]+;
}
```

---

## 5. LEX

The `LEX` section describes how the raw character stream is tokenized. Rules are grouped into **lexer states** (modes). The lexer may maintain a **stack** of active states; some rules can **push**, **pop**, or **set** that stack when they match.

### 5.1 State blocks

**Structure:**

```
LEX {
  state <StateName> [, <StateName> ...] {
    <lex rules>
  }
  // additional state … { … } blocks allowed
}
```

* **`INITIAL`:** At least one state block must include the state name `INITIAL`. That block is the lexer’s entry state. The compiler treats it as the start ruleset.
* **Comma-separated names:** A single block may list several states (`state A, B, C { … }`). Every rule in the body is registered for **each** of those states with the same pattern, priority, role, and stack operation.
* **Multiple blocks:** You may declare additional `state … { … }` groups for other modes. States referenced by `push` or `set` must be declared somewhere in `LEX`.

### 5.2 Rule line

**Syntax:**

```
[<Priority>] <TokenName> -> <Role> : <PatternRef_or_Regex> [<stack-mutation>] [%% <MetaTags> %%] ;
```

* **Priority:** Optional integer (defaults to 0). Higher numbers are tried first within a state.
* **TokenName:** Identifier used from `PARSE` and `PRATT` (and tooling). Lexer token names are identifiers, not quoted strings.
* **Role:** Identifier classifying the token (for example for `IGNORE` in `PARSE`).
* **Pattern / regex:** A name defined in `PATTERN`, or an inline regex literal (`` `...` ``). Inline string literals are not allowed here.
* **Meta tags:** Optional `%% key=value … %%` after the rule (for example `EOF=true` on the end-of-file token).

### 5.3 Stack mutations (optional)

After the pattern (and before `%%` meta, if any), you may append **one** mutation wrapped in **square brackets** `[` … `]`. Inside the brackets, `push`, `pop`, and `set` must be followed by a **parenthesized** argument list (parentheses are not optional).

| Form | Meaning |
|------|---------|
| `[push(State1, State2, …)]` | Push the listed states onto the lexer stack (order is significant for the engine). At least one state name is required inside `()`. |
| `[pop(N)]` | Pop `N` frames (`N` is a decimal integer; use `1` for a single pop). |
| `[set(State1, State2, …)]` | Replace the stack with the given states. At least one state name is required inside `()`. |

Omit the entire `[` … `]` mutation when the match should not change the stack.

### 5.4 Parse, Pratt, and lexer tokens

`PARSE` and `PRATT` refer to tokens by the same identifiers used in `LEX`. A reference is valid whenever that token name is declared by **some** lex rule in **any** state. There is no requirement that the token be defined in `INITIAL`; a token may be produced only after a `push` / `set` (or other mode change) moves the lexer into another state.

At **run time**, the parser can only consume a token if the lexer’s current state (and stack) actually allows that token to match. That is a behavioral contract between your lex table and your grammar, not something the language restricts by limiting parse references to `INITIAL`.

Where the same token name appears in more than one state, pattern and stack metadata must stay consistent across those rules (see lexer validation).

**Example (single state):**

```
LEX {
  state INITIAL {
    10 TokIf     -> keyword    : pat_IfKeyword;
    5  TokIdent  -> identifier : identifier;
    5  TokNumber -> numeric    : `[0-9]+`;
    0  TokWS     -> whitespace : pat_ws;
  }
}
```

**Example (modes + stack):**

```
LEX {
  state INITIAL {
    1 TokStringStart -> string : pat_quote [push(STRING)];
    0 TokWS          -> ws     : pat_ws;
  }
  state STRING {
    1 TokStringEnd   -> string : pat_quote [pop(1)];
    1 TokChar        -> string : pat_string_char;
  }
}
```

---

## 6. PRATT

**Syntax:**
`[local] <ExprName> [sync(Tokens)] { <Categories> }`

Expressions are grouped into a named block. You can optionally restrict visibility with `local` and define error recovery with `sync`.

* **Primary:** A reference to a parse expression.
* **Prefix / Postfix:** Unary operators mapping a symbol to a Node. Require a `precedence` integer.
* **Infix:** Binary operators. Require left and right precedence integers (e.g., `precedence 20 21` for left-associative).
* **Implicit:** Defines implicit operator behavior (e.g., juxtaposition) mapping to a Node with left/right precedence.

**Example:**

```
PRATT {
  local expr {
    primary { 
      EXPRESSION_REFERENCE;
    }
    prefix {
      "-" -> NodeNegate precedence 50;
    }
    postfix {
      "++" -> NodeIncrement precedence 60;
    }
    infix {
      "*" -> NodeMultiply precedence 40 41; // Left-associative
      "/" -> NodeDivide   precedence 40 41;
      "+" -> NodeAdd      precedence 30 31;
      "-" -> NodeSubtract precedence 30 31;
      "=" -> NodeAssign   precedence 11 10; // Right-associative
    }
  }
}
```

*(Note: The generated node names, like `NodeAdd`, will be output to the Lossless Syntax Tree).*

---

## 7. PARSE

The `PARSE` section defines the core syntactic grammar rules. Use the `rule` keyword for each production (see the full example at the end of this section).

**Syntax:**
`rule <RuleName> -> [transparent] <NodeKind> [sync(Tokens)] { <Expressions> };`

### Rule Modifiers
* **transparent:** Prevents the rule from generating its own wrapper node in the LST.
* **sync ( TokList ):** Defines a recovery point for parser error handling.

### Differentiating Rules and Tokens
To map matched tokens into the syntax tree, assign an output node kind using a colon. You can map single tokens only.
* **Token Mapping:** `NodeIdent : TokIdent;`

### Rule Expressions
* **Virtual Tokens:** `virtual TokComma;` (Matches the token but drops it from the LST).
* **Optional:** `( expr )?`
* **Repetition:** `( expr )*` or `( expr )+` or bounded `( expr ){1,3}`
* **Alternation:** `expr1 | expr2`

### Advanced Lookahead and Grouping
* **Predict:** `predict ( offset = TokIf, ... )` forces lookahead disambiguation.
* **Nest (explicit delimiters):** `nest TokOpen TokClose [sync(Tokens)] { body };` matches `TokOpen`, then `body`, then `TokClose`.
* **Nest (named pair):** `nest @PairName [sync(Tokens)] { body };` uses a **`pair`** declaration (§7.2) to supply the open and close lexer tokens. `PairName` is the identifier from the `pair` line.
* **Embed (cross-module handoff):** `embed Alias nest TokOpen TokClose` switches lexer/parser ownership to the embedded module between host delimiters.

### 7.1 Embed handoff (host -> embedded -> host)

Embed syntax:

```
IMPORT {
  embed "sql.lspec" as SQL;
}

PARSE {
  rule EMBED_SQL -> NodeSQLBlock {
    embed SQL nest TokKWSQL TokTripleBackticks
  };
}
```

Compile-time behavior:

1. **Entry injection**: every host lex rule that emits `TokKWSQL` gets a push to `SQL::INITIAL`.
2. **Exit injection**: the compiler synthesizes a max-priority rule in `SQL::INITIAL` that matches the host close-token pattern (`TokTripleBackticks`), emits that close token, and executes `pop(1)`.
3. **Parser lowering**: the embed statement lowers to `open token -> embedded PROGRAM -> close token`.

Validation guardrails:

* `V_IMP009`: embed alias is not declared as an `embed` import.
* `V_IMP010`: embed entry token does not exist in reachable host lexer rules.
* `V_IMP011`: embed exit pattern can be shadowed inside embedded root lexer rules.

### 7.2 Pair declarations

Inside the `PARSE` section (alongside ordinary rules), you may declare **token pairs** for reuse in `nest`:

**Syntax:**

```
pair <PairName> <OpenTok> <CloseTok>;
```

* **PairName:** Identifier; must be unique among pairs and must not collide with tokens, patterns, parse rules, or Pratt names.
* **OpenTok / CloseTok:** Lexer token names (as in `LEX`) for the opening and closing delimiter.

The declaration line ends with `;` (after optional layout tokens as in the meta-grammar).

### 7.3 Nest with a pair reference

After declaring `pair P TokOpen TokClose`, write:

```
nest @P {
  …
}
```

The lexer must produce your `TokPairReference` (or equivalent) for `@P`; the name after `@` must match **PairName** exactly. The parser lowers this to the same nested structure as `nest TokOpen TokClose { … }`.

**Known edge case (metascopes):**
Metascope behavior currently works reliably only when the relevant construct is marked as a `nest`. In other situations it can be inconsistent (hit and miss). Pull requests to improve this are welcome; this is not planned for near-term maintainer work because the fix is complex and the impact is relatively minor.

### 7.4 Parse templates (parameterized fragments)

A **template** is a named, parameterized parse fragment. It does not become its own parse rule in the lowered grammar; **call sites are expanded by substitution** (the body is cloned, and each `$parameter` is replaced by the corresponding argument) before compilation.

**Declaration**

```
template <TemplateName> ( <param> : <Type> [, <param> : <Type> ...] ) {
  <parse expression>
}
```

* **TemplateName:** Identifier; must not collide with a lexer token, pattern, parse rule, pair, or Pratt name (same global symbol rules as pairs and rules).
* **Parameters:** Each formal is a lexer `TokParameter` name (e.g. `$element`) followed by `:` and a type keyword.

**Parameter types** (argument at each position must match):

| Type keyword   | Argument at call site |
|----------------|------------------------|
| `Token`        | Lexer token identifier (declared in `LEX`). |
| `Node`         | Output node kind string (same namespace as rule headers / mapping LHS names). |
| `Rule`         | Parse rule name. |
| `Pair`         | Pair reference `@PairName`. |
| `PrattExpr`    | Pratt expression name. |

**Call syntax**

Inside a parse expression, a template is invoked only with the **`call`** keyword (so it cannot be confused with a bare rule reference followed by grouping):

```
call TemplateName(Arg1, Arg2, …)
```

Arguments are comma-separated. Types must match the template signature in order; arity must match.

**Why `call` is required:** A bare identifier followed by parentheses is still ordinary parse syntax (e.g. grouping, optional `( … )?`, or concatenation with a nested group). The language does **not** treat `SomeName(Arg1, Arg2)` as a template call. Only the `call` form is a template invocation, so the parser and validator need no ambiguity rules between “rule reference + grouping” and “template call.”

**Scoping**

* `$parameter` references are **only** valid inside the template body (between `{` and `}`).
* Template bodies may call other templates using `call OtherTemplate(...)`, but the **template call graph must be acyclic** (no cycles, including a template calling itself). The validator reports a clear path on violation.

**Example**

```
template TEMPLATE_LIST($elementTk : Token, $elementNode : Node, $separator : Token) {
	$elementNode : $elementTk
	(
		virtual $separator
		$elementNode : $elementTk
	)*
}

rule LIST_TEST -> NodeList {
	call TEMPLATE_LIST(TokStringLiteral, NodeListElement, TokComma)
}
```

**Example failures**

* Wrong arity or wrong argument type → validation error at the call.
* Using a template name without `call …(...)` where a rule reference is expected → error (use `call` with arguments or use a parse rule).
* Template A calls B calls A → cycle error.

The maintainer **generator** may emit synthetic `GenTempl_*` templates when fingerprinting finds repeated structure in the lowered grammar, to shrink generated `.lspec` text; call sites are emitted as `call GenTempl_*(...)` and must still parse and validate like hand-written ones.

**Full PARSE example (rules, pairs, nest):**

```
PARSE {
  IGNORE { whitespace };

  // The entry point must be named PROGRAM
  rule PROGRAM -> NodeProgram {
    ( Statement )*
    virtual EOF
  };

  rule Statement -> NodeStatement {
    IfStatement | ExpressionStatement
  };

  rule IfStatement -> NodeIf sync ( TokSemicolon ) {
    virtual TokIf
    virtual TokOpenParen
    NodeCondition : expr  // References the PRATT block
    virtual TokCloseParen
    Body : Block
  };

  rule Block -> NodeBlock {
    nest TokOpenBrace TokCloseBrace {
      ( Statement )*
    };
  };

  pair Braces TokOpenBrace TokCloseBrace;
  rule ParenBlock -> NodeParenBlock {
    nest @Braces {
      Expression
    };
  };
}
```