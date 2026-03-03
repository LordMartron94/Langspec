# TODO

In this file I will outline the things I still need/want to do for LSPEC.

## PRIORITIES - PARSING PHASE & LSPEC DEFINITION

### Priority 1

- [x] Fix Editor Engine causing duplicate includes. 
	- FIXED ON 03 March 2026 @ 06.35pm
- [x] Fix Editor Engine bug where a certain override causes subsequent content to be invalid.
	- FIXED ON 03 March 2026 @ 06.52pm
- [ ] Complete Pattern section syntax.
	- [ ] 

### Priority 2

### Priority 3

- [ ] Macros
- [x] Inside Pragma section, scope attribute-value based on actual type (boolean, string, etc.).
	- DONE ON 03 March 2026 @ 07.26pm
	- [x] Add tokens for boolean.
		- DONE ON 03 March 2026 @ 07.10pm
	- [x] Update editor IR generator to support overrides for specific tokens within a node.
		- DONE ON 03 March 2026 @ 07.02pm
	- [x] Fix issue in EditorIR where token lexing priorities aren't taken into account.
		- FIXED ON 03 March 2026 @ 07.26pm

### Priority 4

- [ ] Add extension support for pragmas (including validation)

### Priority 5

---

## FUTURE

- [ ] LSpec support for EBNF (.lspec file for EBNF with compiler support) 
- [ ] Refactor Lexer to support tables for lexing (i.e., true becomes identifier IF it does not exist inside table)
