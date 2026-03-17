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

## 4. PATTERN

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

The `LEX` section dictates how the raw character stream is tokenized. 

**Syntax:**
`<Priority> <TokenName> -> <Role> : <PatternRef_or_Regex> ; [%% MetaTags %%]`

* **Priority:** Optional Integer (defaults to 0). Higher numbers are attempted first.
* **TokenName:** The identifier used in the `PARSE` and `PRATT` sections.
* **Role:** A string identifier classifying the token (useful for the `IGNORE` directive).
* **Pattern / Regex:** Must be a reference to a `PATTERN` identifier or an inline regex literal (`` `...` ``). **Inline string literals are not permitted.**

**Example:**

```
LEX {
  // Must use pattern references or regex, NEVER raw strings like "if"
  10 TokIf    -> keyword : pat_IfKeyword;
  5 TokIdent  -> identifier : identifier; 
  5 TokNumber -> numeric    : `[0-9]+`;   
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