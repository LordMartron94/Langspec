# TODO

In this file I will outline the things I still need/want to do for LSPEC.

## PRIORITIES - PARSING PHASE & LSPEC DEFINITION

### Priority 1

- [x] Fix Editor Engine causing duplicate includes. 
	- FIXED ON 03 March 2026 @ 06.35pm
- [x] Fix Editor Engine bug where a certain override causes subsequent content to be invalid.
	- FIXED ON 03 March 2026 @ 06.52pm
- [X] Complete Pattern section syntax. -- DONE ON 04 March 2026 @ 09.32pm
	- [X] Plus -- DONE ON 04 March 2026 @ 07.25pm
	- [X] Optional -- DONE ON 04 March 2026 @ 09.15pm
	- [X] Repeat(min, max) -- DONE ON 04 March 2026 @ 09.32pm
	- [X] Negation -- DONE ON 04 March 2026 @ 07.25pm
	- [X] Grouping -- DONE ON 04 March 2026 @ 07.25pm
- [ ] Test LSpec LSpec lexemes against Go compiler
	- [ ] Add all current LSpec patterns & lexemes to input.lspec
	- [ ] Build initial compiler stage for compiling patterns to Regula
	- [ ] Build compiler stage for defining tokens referencing those patterns or regex literals
	- [ ] Feed input.lspec into compiler and check output against go output

### Priority 2

- [x] Support private variables that are local to Pattern section.
	- DONE ON 04 March 2026 @ 12.03am
- [ ] Extend validation
	- [ ] Cyclic patterns/variables (i.e., variables cannot refer to themselves)
	- [ ] Validate unreachable patterns (emit warning or info)
	- [ ] Detect ambiguous token matches
	- [ ] Warn when two tokens produce identical patterns
	- [ ] Detect tokens shadowed by higher priority tokens
- [ ] Import system

### Priority 3

- [ ] Grammar macros (for users to define their own factories externally and such)
- [x] Inside Pragma section, scope attribute-value based on actual type (boolean, string, etc.).
	- DONE ON 03 March 2026 @ 07.26pm
	- [x] Add tokens for boolean.
		- DONE ON 03 March 2026 @ 07.10pm
	- [x] Update editor IR generator to support overrides for specific tokens within a node.
		- DONE ON 03 March 2026 @ 07.02pm
	- [x] Fix issue in EditorIR where token lexing priorities aren't taken into account.
		- FIXED ON 03 March 2026 @ 07.26pm
- [ ] Editor IR: Add support to override nodes depending on token/node context (previous and subsequent nodes/tokens)
	- This can help with for example scoping a private variable differently based on the fact that a local keyword appears before.

### Priority 4

- [ ] Add extension support for pragmas (including validation)

### Priority 5

---

## FUTURE

- [ ] LSpec support for EBNF (.lspec file for EBNF with compiler support)
	- [ ] EBNF emitter for lspec too
	- [ ] as well as a .lspec emitter for ebnf files 
- [ ] Refactor Lexer to support tables for lexing (i.e., true becomes identifier IF it does not exist inside table)
- [ ] Design an LSP server for LSpec (including package for Sublime)