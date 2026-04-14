package sublime

import (
	"cmp"
	"fmt"
	"sort"
	"strings"

	"langspec/editor"
	"lexarch"
	"syntaxa"
)

/*
PeekEmitConfig supplies lexer reachability data so the Sublime emitter can turn
grammar PeekAfterMatch constraints into positive lookaheads. The editor IR carries
only []syntaxa.Lookahead; regex composition happens here, not in editor.
*/
type PeekEmitConfig[TObservation cmp.Ordered, TToken ~uint32, TTokenRole comparable] struct {
	LexerReach   map[string]map[string]bool
	RulesByState map[string][]editor.LexingRule[TObservation, TToken, TTokenRole]
	// GapPattern is inserted before the next token’s regex inside each (?=...), e.g. `\s*`.
	// Empty defaults to `\s*`.
	GapPattern string
}

func gapPatternOrDefault(gap string) string {
	if gap == "" {
		return `\s*`
	}
	return gap
}

/*
applyPeekAfterMatchToRegex appends (?=...) groups for PeekAfterMatch constraints.
Offset-0 entries use the same parse context for resolving the next token’s lexer rule.
Consecutive offsets 0,1,2,… are folded into one (?=gap+R0 gap+R1 …) so multi-token
predict guards (see lowering.peekAfterMatchFromAdvTerminals) distinguish Sublime rules.
All offsets still resolve rules via afterParseContextID (approximation if lexer state
changes mid-chain).
*/
func applyPeekAfterMatchToRegex[TObservation cmp.Ordered, TToken ~uint32, TTokenRole comparable](
	baseRegex string,
	peek []syntaxa.Lookahead[lexarch.TokenKind],
	afterParseContextID string,
	opt *PeekEmitConfig[TObservation, TToken, TTokenRole],
) (string, error) {
	if opt == nil || len(peek) == 0 || afterParseContextID == "" {
		return baseRegex, nil
	}

	cp := append([]syntaxa.Lookahead[lexarch.TokenKind](nil), peek...)
	sort.Slice(cp, func(i, j int) bool {
		if cp[i].Offset != cp[j].Offset {
			return cp[i].Offset < cp[j].Offset
		}
		return cp[i].Expected < cp[j].Expected
	})

	gap := gapPatternOrDefault(opt.GapPattern)
	var b strings.Builder
	b.WriteString(baseRegex)

	used := make([]bool, len(cp))

	resolveRegex := func(tok lexarch.TokenKind) (string, bool, error) {
		rule, ok, err := editor.ResolveLexingRuleAtParseContext(
			afterParseContextID, tok, opt.LexerReach, opt.RulesByState,
		)
		if err != nil || !ok {
			return "", ok, err
		}
		nextRe, err := rule.Pattern.ToRegEx()
		if err != nil {
			return "", false, fmt.Errorf("sublime peek lookahead: pattern to regex: %w", err)
		}
		return nextRe, true, nil
	}

	writeSingle := func(tok lexarch.TokenKind) error {
		nextRe, ok, err := resolveRegex(tok)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		b.WriteString("(?=")
		b.WriteString(gap)
		b.WriteString(nextRe)
		b.WriteString(")")
		return nil
	}

	writeChain := func(chain []syntaxa.Lookahead[lexarch.TokenKind]) error {
		var frags []string
		for _, l := range chain {
			nextRe, ok, err := resolveRegex(l.Expected)
			if err != nil {
				return err
			}
			if !ok {
				for _, m := range chain {
					if err := writeSingle(m.Expected); err != nil {
						return err
					}
				}
				return nil
			}
			frags = append(frags, gap+nextRe)
		}
		b.WriteString("(?=")
		b.WriteString(strings.Join(frags, ""))
		b.WriteString(")")
		return nil
	}

	for {
		minI := -1
		for i := range cp {
			if used[i] {
				continue
			}
			if minI == -1 || cp[i].Offset < cp[minI].Offset {
				minI = i
			}
		}
		if minI == -1 {
			break
		}

		if cp[minI].Offset > 0 {
			if err := writeSingle(cp[minI].Expected); err != nil {
				return "", err
			}
			used[minI] = true
			continue
		}

		start := minI
		for i := range cp {
			if used[i] || cp[i].Offset != 0 {
				continue
			}
			if cp[i].Expected < cp[start].Expected {
				start = i
			}
		}

		chain := []syntaxa.Lookahead[lexarch.TokenKind]{cp[start]}
		used[start] = true
		want := 1
		for want < 64 {
			pick := -1
			for i := range cp {
				if used[i] || cp[i].Offset != want {
					continue
				}
				if pick == -1 || cp[i].Expected < cp[pick].Expected {
					pick = i
				}
			}
			if pick == -1 {
				break
			}
			chain = append(chain, cp[pick])
			used[pick] = true
			want++
		}

		if len(chain) >= 2 {
			if err := writeChain(chain); err != nil {
				return "", err
			}
		} else {
			if err := writeSingle(chain[0].Expected); err != nil {
				return "", err
			}
		}
	}

	return b.String(), nil
}
