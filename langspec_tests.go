package langspec

import (
	"fmt"
	"os"
	"testing"

	ftesting "foundation/testing"

	"autarch/pattern"
	"lexarch"
	"memcore"
	"memforge"

	"syntaxa"
	"syntaxa/pratt"
	"syntaxa/rd"
)

// ============================================================
// Types (IDENTICAL to Syntaxa test)
// ============================================================

type LexerState int

const NormalState LexerState = 1

type TokenRole int

const (
	TriviaRole TokenRole = iota + 1
	StructuralRole
)

type Token int

const (
	WhitespaceTok Token = iota + 1
	NumberTok
	IdentTok

	PlusTok
	StarTok
	LParenTok
	RParenTok
	SemicolonTok

	ErrorTok
	EOFToken
)

type NodeKind int

const (
	RootNode NodeKind = iota + 1
	StmtNode
	BinaryExpr
	NumberExpr
	IdentExpr
	CallExpr
)

// ============================================================
// LangSpec builder (grammar = EXACT Syntaxa grammar)
// ============================================================

func buildLangSpec() *LangSpec[rune, Token, TokenRole, LexerState, NodeKind] {

	// ---------- Lexer ----------

	lexer := LexerSpecCreate[rune, Token, TokenRole, LexerState](
		ErrorTok,
		EOFToken,
		NormalState,
		lexarch.NewlineDetectorRune(),
	)

	rules := lexarch.LexingRulesetCreate[rune, Token, TokenRole](
		lexarch.TokenResolutionStepPriority[Token],
	)

	number := pattern.Digit.Plus()
	ident := pattern.Lower.Plus()

	ws := pattern.AnyOf(
		pattern.Literal(' '),
		pattern.Literal('\n'),
		pattern.Literal('\t'),
	).Plus()

	rules.WithRule(ws, WhitespaceTok, TriviaRole)
	rules.WithRule(number, NumberTok, StructuralRole)
	rules.WithRule(ident, IdentTok, StructuralRole)

	rules.WithRule(pattern.Literal('+'), PlusTok, StructuralRole)
	rules.WithRule(pattern.Literal('*'), StarTok, StructuralRole)
	rules.WithRule(pattern.Literal('('), LParenTok, StructuralRole)
	rules.WithRule(pattern.Literal(')'), RParenTok, StructuralRole)
	rules.WithRule(pattern.Literal(';'), SemicolonTok, StructuralRole)

	lexer.WithRuleset(NormalState, *rules)

	// ---------- Pratt ----------

	p := pratt.PrattParserCreate[rune, Token, TokenRole, NodeKind, LexerState]()

	p.RegisterPrefix(NumberTok, func(ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {
		lex := ctx.Consume()
		n := ctx.CreateASTNode()
		n.NodeKind = NumberExpr
		n.Tokens = append(n.Tokens, lex)
		return n
	})

	p.RegisterPrefix(IdentTok, func(ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {
		lex := ctx.Consume()
		n := ctx.CreateASTNode()
		n.NodeKind = IdentExpr
		n.Tokens = append(n.Tokens, lex)
		return n
	})

	p.RegisterPrefix(LParenTok, func(ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {
		ctx.Consume()
		e := p.ParseExpr(ctx, 0)
		ctx.Match(RParenTok)
		return e
	})

	parseBinary := func(
		ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind],
		left *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind],
		rbp int,
	) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		op := ctx.Consume()
		right := p.ParseExpr(ctx, rbp)

		n := ctx.CreateASTNode()
		n.NodeKind = BinaryExpr
		n.Children = []*syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind]{left, right}
		n.Tokens = append(n.Tokens, op)

		left.Parent = n
		right.Parent = n
		return n
	}

	p.RegisterInfix(PlusTok, 10, pratt.Left, parseBinary)
	p.RegisterInfix(StarTok, 20, pratt.Left, parseBinary)

	p.RegisterPostfix(LParenTok, 30, func(
		ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind],
		left *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind],
	) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		ctx.Consume()
		ctx.Match(RParenTok)

		n := ctx.CreateASTNode()
		n.NodeKind = CallExpr
		n.Children = []*syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind]{left}
		left.Parent = n
		return n
	})

	// ---------- Grammar (IDENTICAL to Syntaxa test) ----------

	selector := func(selectCtx syntaxa.SelectRuleContext[rune, Token, TokenRole]) syntaxa.ParserRule[rune, Token, TokenRole, LexerState, NodeKind] {
		expr := func(execCtx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) (*syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind], bool) {
			execCtx.PushSkipRoles(TriviaRole)
			defer execCtx.PopSkipRoles()
			n := p.ParseExpr(execCtx, 0)
			return n, n != nil
		}

		stmt := rd.Sequence(
			expr,
			rd.TokenMatch[rune, Token, TokenRole, LexerState, NodeKind](SemicolonTok, StmtNode),
		)
		return stmt
	}

	return LangSpecCreate(
		lexer,
		ParserSpecCreate(RootNode, selector),
	)
}

// ============================================================
// End-to-end LangSpec test
// ============================================================

func TestLangSpecEndToEnd(t *testing.T) {

	spec := buildLangSpec()

	alloc := memforge.DynamicLinearAllocatorCreateFunction(
		uint64(memcore.KiloByte),
		func(c, n uint64) uint64 { return max(c*2, n) },
	)
	defer memforge.DynamicLinearAllocatorDestroy(alloc)

	cfg := LangParserConfigurationCreate(
		spec,
		func(sz, align uint64) memcore.MarkRaw {
			return memforge.DynamicLinearAllocatorMallocUnsafe(alloc, sz, align)
		},
	)

	source := "foo() + 2 * 3;"

	tmp, _ := os.CreateTemp("", "langspec-*.txt")
	defer os.Remove(tmp.Name())
	tmp.WriteString(source)
	tmp.Close()

	for _, streaming := range []bool{false, true} {

		parser := LangParserCreate(cfg)

		root, errs, err := LangParserParseFile(
			parser,
			tmp.Name(),
			nil,
			streaming,
		)

		if err != nil {
			t.Fatal(err)
		}

		validateAST(t, root, errs)
	}
}

// ============================================================
// AST validation (IDENTICAL semantics)
// ============================================================

func validateAST(
	t *testing.T,
	root *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind],
	errs *syntaxa.SyntaxErrors,
) {
	ftesting.Assert(
		errs == nil || !errs.HasErrors(),
		func() string {
			if errs == nil {
				return "syntax errors: <nil errs>"
			}
			return fmt.Sprintf("syntax errors: %+v", errs.Errors)
		}(),
		"no syntax errors",
		t,
	)

	// Root must contain exactly one statement.
	if root == nil {
		ftesting.Assert(false, "root AST node is nil", "root ok", t)
		return
	}

	ftesting.Assert(
		len(root.Children) == 1,
		fmt.Sprintf("expected single statement, got %d", len(root.Children)),
		"single statement ok",
		t,
	)
	if len(root.Children) < 1 {
		return
	}

	stmt := root.Children[0]
	if stmt == nil {
		ftesting.Assert(false, "stmt node is nil", "stmt ok", t)
		return
	}

	// Statement must have at least one child (the expression).
	ftesting.Assert(
		len(stmt.Children) >= 1,
		fmt.Sprintf("expected stmt to have expr child, got %d children", len(stmt.Children)),
		"stmt has expr",
		t,
	)
	if len(stmt.Children) < 1 {
		return
	}

	expr := stmt.Children[0]
	if expr == nil {
		ftesting.Assert(false, "expr node is nil", "expr ok", t)
		return
	}

	// Expression must be a binary (+) with exactly 2 children.
	ftesting.Assert(
		expr.NodeKind == BinaryExpr,
		fmt.Sprintf("expected + at root (BinaryExpr), got kind=%v childCount=%d",
			expr.NodeKind, len(expr.Children),
		),
		"binary root ok",
		t,
	)
	if expr.NodeKind != BinaryExpr {
		return
	}

	ftesting.Assert(
		len(expr.Children) == 2,
		fmt.Sprintf("binary arity wrong: expected 2 children, got %d", len(expr.Children)),
		"binary arity ok",
		t,
	)
	if len(expr.Children) < 2 {
		return
	}

	left := expr.Children[0]
	right := expr.Children[1]
	if left == nil || right == nil {
		ftesting.Assert(false, "binary children contain nil", "binary children ok", t)
		return
	}

	// Left must be a call.
	ftesting.Assert(
		left.NodeKind == CallExpr,
		fmt.Sprintf("expected call on left, got kind=%v childCount=%d", left.NodeKind, len(left.Children)),
		"call ok",
		t,
	)

	// Right must be multiplication binary expr.
	ftesting.Assert(
		right.NodeKind == BinaryExpr,
		fmt.Sprintf("expected multiplication on right (BinaryExpr), got kind=%v childCount=%d",
			right.NodeKind, len(right.Children),
		),
		"precedence ok",
		t,
	)
}
