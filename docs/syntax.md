# LangSpec DSL Syntax Reference

The LangSpec Domain Specific Language (DSL) defines the complete lexical and syntactic structure of a target language in a single `.lspec` file. 

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

## 3. PRAGMA

The `PRAGMA` section defines tooling and compiler directives. The block key **must** begin with the `tool` keyword.

**Syntax:**

    PRAGMA {
      tool.<tool_name> {
        key = value;
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
}
```

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

The `PARSE` section defines the core syntactic grammar rules. 

**Syntax:**
`<RuleName> -> [transparent] <NodeKind> [sync(Tokens)] { <Expressions> };`

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
* **Nest:** `nest TokOpen TokClose [sync(Tokens)] { body };` ensures proper delimited grouping, with optional internal sync recovery.

**Example:**

```
PARSE {
  IGNORE { whitespace };

  // The entry point must be named PROGRAM
  PROGRAM -> NodeProgram {
    ( Statement )*
    virtual EOF
  };

  Statement -> NodeStatement {
    IfStatement | ExpressionStatement
  };

  IfStatement -> NodeIf sync ( TokSemicolon ) {
    virtual TokIf
    virtual TokOpenParen
    NodeCondition : expr  // References the PRATT block
    virtual TokCloseParen
    Body : Block
  };

  Block -> NodeBlock {
    nest TokOpenBrace TokCloseBrace {
      ( Statement )*
    };
  };
}
```