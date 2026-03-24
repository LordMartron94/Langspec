# Developer TODO & Architectural Scratchpad

> **WARNING FOR USERS:**
> Close this file. This is an internal developer scratchpad, not public documentation. It represents raw thoughts, planned refactors, and incomplete architectural shifts. It is guaranteed to be out of sync with the main branch. Relying on anything written here will break your implementation.

## Priority System
* **[P0] Critical Path:** The engine is actively lying or failing. Blocks fundamental usage. Fix immediately.
* **[P1] Strategic Architecture:** Core features required for LangSpec to mature into a production-grade ecosystem.
* **[P2] Ergonomics & QoL:** Boilerplate reduction and tooling. Do not touch until P0 and P1 are stable.
* **[P3] Backlog/Research:** Ideas that sound good but need strict scope-checking to avoid bloat.

---

## [P0] Disambiguation & Complex Lookahead Logic
* **The Problem:** The current `predict` modifier is a shallow hack. It applies fixed-offset checks that immediately break when optional prefixes (like `(export | private)?`) shift the token stream. 
* **The Goal:** Define a robust resolution for LL(1) FIRST-set collisions that doesn't rely on fragile, hardcoded integers.
* **Action Items:**
  - [ ] **Decision:** Do we formally build LL(k) / boolean lookahead logic into `autarch/pattern` (State-space explosion risk), OR do we strictly enforce LL(1) left-factoring in the DSL?
  - [ ] If keeping `predict`, upgrade the syntax to support condition blocks instead of raw offsets, e.g., `predict { Peek(1) == TokTheme }`.
## [P0] Clarify or Extend behavior for nesting logic.
- **The Problem:** currently it is unclear whether the nest automatically syncs/recovers on its closing token or not. Make this clear.


## [P1] Grammar Import and Export Mechanism
* **The Problem:** Monolithic `.lspec` files do not scale. Real-world languages require modularity (e.g., importing a standard regex library or a base JSON grammar into a larger config language).
* **The Goal:** Allow `.lspec` files to safely compose and inherit from one another.
* **Action Items:**
  - [ ] Design the `import "path/to/grammar.lspec"` syntax.
  - [ ] Define namespace resolution. (If I import `Regex.lspec`, do its tokens prefix with `Regex::` or pollute the global scope?)
  - [ ] Update Stage 0 Validation (Symbol Binding) to handle cross-file reference resolution and detect circular imports.
  - [ ] Ensure the LST generation tracks which file a node originated from for accurate error reporting.

## [P2] Templating / Macro System
* **The Problem:** Writing repetitive structural rules (like delimited lists, binary expressions, or identical nest wrappers) inflates the `.lspec` file and invites copy-paste errors.
* **The Goal:** Introduce a hygienic macro system to abstract boilerplate without destroying the readability of the grammar.
* **Action Items:**
  - [ ] Design macro definition syntax. E.g., `macro CommaSeparated(Rule) -> (Rule (TokComma Rule)*)?`
  - [ ] Implement a macro expansion pass *before* AST validation (Stage 0).
  - [ ] **Risk Mitigation:** Ensure that LST debug dumps and syntax errors map back to the *macro invocation line*, not the expanded internal AST, otherwise developers will have no idea why their grammar failed.

## [P3] Backlog
* [ ] Add toolchain for automatic .tmPreference comments for grammars