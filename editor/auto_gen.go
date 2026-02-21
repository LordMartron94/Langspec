package editor

import (
	"cmp"
	"fmt"
	"lexarch"
	"sort"
)

// ============================================================
// REQUIRED READ-ONLY VIEW (implemented by lexarch or adapter)
// ============================================================

/* TokenPatternExtractor takes in a token and outputs its equivalent RegEx pattern. */
type TokenPatternExtractor[TToken comparable] func(token TToken) string

// ============================================================
// EDITOR IR (your existing types)
// ============================================================

/* TokenStyle encapsulates a generic style for syntax render generation. */
type TokenStyle[TToken comparable] struct {
	Token    TToken
	Scope    string
	Priority int
}

/* Context encapsulates contexts for syntax render generation. */
type Context[TToken comparable] struct {
	Name    string
	EnterOn TToken
	ExitOn  TToken
}

// ============================================================
// GENERATED RESULT
// ============================================================

/*
GeneratedSyntax is the editor-IR output derived from lexer rules.
For now (single lexer state), Contexts will be empty unless you add them manually.
*/
type GeneratedSyntax[TToken comparable] struct {
	TokenStyles        []TokenStyle[TToken]
	ContextDefinitions []Context[TToken] // empty for single-state auto-gen
	PatternExtractor   TokenPatternExtractor[TToken]
}

// ============================================================
// AUTOGEN (SINGLE-STATE)
// ============================================================

/*
BuildGeneratedSyntaxFromRulesetSingleState derives editor syntax structures from a single ruleset.

Inputs:
- ruleset: read-only view of lexing rules
- roleScopes: mapping from token role -> Sublime/TextMate scope
- toRegex: converts a pattern AST to regex

Outputs:
- TokenStyles: unique per token (best priority wins)
- PatternExtractor: token -> regex (best priority wins)
- ContextDefinitions: empty (single-state)
*/
func BuildGeneratedSyntaxFromRulesetSingleState[
	TObservation cmp.Ordered,
	TToken comparable,
	TTokenRole comparable,
](
	ruleset *lexarch.LexingRuleset[TObservation, TToken, TTokenRole],
	roleScopes map[TTokenRole]string,
	tokenScopes map[TToken]string,
	scopeSuffix string,
) (*GeneratedSyntax[TToken], error) {

	if ruleset == nil {
		return nil, fmt.Errorf("ruleset is nil")
	}
	if roleScopes == nil {
		roleScopes = map[TTokenRole]string{}
	}
	if tokenScopes == nil {
		tokenScopes = map[TToken]string{}
	}

	rules := ruleset.GetRules()
	if len(rules) == 0 {
		return &GeneratedSyntax[TToken]{
			TokenStyles:        []TokenStyle[TToken]{},
			ContextDefinitions: []Context[TToken]{},
			PatternExtractor:   func(token TToken) string { return "" },
		}, nil
	}

	// Best rule per token (priority, then first-seen stable)
	type bestRule struct {
		regex    string
		scope    string
		priority int
		seenAt   int
	}

	bestByToken := make(map[TToken]bestRule, len(rules))

	for i, r := range rules {
		regex, err := r.PatternToRegEx()
		if err != nil {
			return nil, fmt.Errorf(
				"pattern->regex failed (token=%v, role=%v): %w",
				r.Token,
				r.Role,
				err,
			)
		}
		if regex == "" {
			return nil, fmt.Errorf(
				"pattern->regex returned empty regex (token=%v, role=%v)",
				r.Token,
				r.Role,
			)
		}

		// -------------------------------
		// Resolve scope (token override > role fallback)
		// -------------------------------

		scope := tokenScopes[r.Token]

		if scope == "" {
			scope = roleScopes[r.Role]
		}
		if scope == "" {
			scope = "source"
		}

		scope = scope + scopeSuffix

		cur, exists := bestByToken[r.Token]
		if !exists {
			bestByToken[r.Token] = bestRule{
				regex:    regex,
				scope:    scope,
				priority: r.Priority,
				seenAt:   i,
			}
			continue
		}

		// Higher priority wins; stable on ties
		if r.Priority > cur.priority {
			bestByToken[r.Token] = bestRule{
				regex:    regex,
				scope:    scope,
				priority: r.Priority,
				seenAt:   cur.seenAt,
			}
		}
	}

	// Deterministic order: priority desc, then original position
	type styleRow struct {
		token    TToken
		scope    string
		priority int
		seenAt   int
		regex    string
	}

	rows := make([]styleRow, 0, len(bestByToken))
	for tok, br := range bestByToken {
		rows = append(rows, styleRow{
			token:    tok,
			scope:    br.scope,
			priority: br.priority,
			seenAt:   br.seenAt,
			regex:    br.regex,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].priority != rows[j].priority {
			return rows[i].priority > rows[j].priority
		}
		return rows[i].seenAt < rows[j].seenAt
	})

	tokenStyles := make([]TokenStyle[TToken], 0, len(rows))
	tokenRegex := make(map[TToken]string, len(rows))

	for _, row := range rows {
		tokenStyles = append(tokenStyles, TokenStyle[TToken]{
			Token:    row.token,
			Scope:    row.scope,
			Priority: row.priority,
		})
		tokenRegex[row.token] = row.regex
	}

	extractor := func(token TToken) string {
		return tokenRegex[token]
	}

	return &GeneratedSyntax[TToken]{
		TokenStyles:        tokenStyles,
		ContextDefinitions: []Context[TToken]{}, // single-state
		PatternExtractor:   extractor,
	}, nil
}
