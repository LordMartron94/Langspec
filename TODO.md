# Developer TODO & Architectural Scratchpad

> **WARNING:** Internal developer scratchpad. Not public documentation. Guaranteed to be out of sync. Relying on this will break your implementation.

## Priority System
* **[P0] Critical Path:** The engine is failing or lying. Blocks basic usage.
* **[P1] Strategic Architecture:** Core features needed for production-grade viability.
* **[P2] DSL Ergonomics & QoL:** Eliminating boilerplate and friction.
* **[P3] Backlog/Research:** Unscoped ideas. 

---

## [P0] Critical Path
* **Disambiguation & Complex Lookahead (`predict`)**
  * *Status:* Current fixed-offset `predict` is a fragile hack that breaks on optional prefixes.
  * *Action:* Decide between LL(k)/boolean lookahead in `autarch/pattern` OR strictly enforced LL(1) left-factoring.
  * *Action:* If keeping `predict`, upgrade to condition blocks (e.g., `predict { Peek(1) == TokTheme }`).
* **Nesting Recovery Semantics**
  * *Status:* Unclear if `nest` automatically syncs/recovers on its closing token. 
  * *Action:* Define and document hard recovery rules for nested boundaries.

## [P1] Strategic Architecture
* **Slotted Architecture Transition (Eliminate "Role-as-Kind" Bloat)**
  * *Status:* The current grammar relies on single-child wrapper nodes (e.g., `NodeRGBRed`, `NodeAssignTarget`) to define structural meaning. This causes severe `NodeKind` enum bloat, deep tree allocations, and forces a constant "unwrapping" tax on the client.
  * *Action:* Upgrade `syntaxa` rule combinators and `RuleResult` to natively populate and propagate slot assignments.
  * *Action:* Introduce slot-binding syntax in LSpec (e.g., `condition: CONDITION_EXPR` or `@condition=CONDITION_EXPR`).
  * *Action:* Rewrite the ruleforge grammar to emit flat, generic shapes (`NodeIntLiteral`, `NodeIdentifier`) that are mapped directly to parent slots, drastically reducing the AST depth.
* **Grammar Composition (Imports/Exports)**
  * *Status:* Monolithic `.lspec` files do not scale.
  * *Action:* Design `import "path/to/grammar.lspec"` syntax.
  * *Action:* Define namespace isolation (e.g., `Regex::Tok` vs global pollution).
  * *Action:* Upgrade Stage 0 Validation to resolve cross-file references and block circular imports.
  * *Action:* Ensure LST generation tracks file origin for cross-file error reporting.

## [P2] DSL Ergonomics & QoL (LSpec Redesign)
* **Eliminate `transparent` Wrapper Boilerplate**
  * *Status:* Users are forced to invent dummy grammar IDs (`gr_VSTMT`) just to apply `sync` modifiers to sequences.
  * *Action:* Allow modifiers directly on anonymous inline blocks.
  * *Constraint:* Strictly enforce modifier ownership to prevent ambiguity (e.g., block conflicting `sync` definitions from existing on both a reference and its referee).
* **Shorthand for `nest` Boundaries**
  * *Status:* `nest TokBraceOpen TokBraceClose` is too verbose.
  * *Action:* Introduce global token pairs or shorthand syntax.
* **Macro System**
  * *Status:* Repetitive structural rules (delimited lists, binary expressions) inflate files.
  * *Action:* Design hygienic macro syntax (e.g., `macro CommaSep(Rule) -> (Rule (TokComma Rule)*)?`).
  * *Action:* Map expansion errors back to the invocation line, not the internal AST.

## [P3] Backlog
* ~~Toolchain for automatic `.tmPreferences` comments generation.~~ (tm_comments toolchain + embedded templates)