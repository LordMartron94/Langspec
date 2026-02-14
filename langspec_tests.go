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
// Types
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

	ErrorNode
)

// ============================================================
// LangSpec builder (SAFE API ONLY)
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

	// ---------- Pratt (Editor-safe) ----------

	p := pratt.PrattParserCreate[rune, Token, TokenRole, NodeKind, LexerState]()

	p.RegisterPrefix(NumberTok, func(ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		lex := ctx.Consume()
		node := ctx.Editor.NewNode(NumberExpr)
		ctx.Editor.AddToken(node, lex)
		return node
	})

	p.RegisterPrefix(IdentTok, func(ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		lex := ctx.Consume()
		node := ctx.Editor.NewNode(IdentExpr)
		ctx.Editor.AddToken(node, lex)
		return node
	})

	p.RegisterPrefix(LParenTok, func(ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind]) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		ctx.Consume()
		expr := p.ParseExpr(ctx, 0)
		ctx.Match(RParenTok)
		return expr
	})

	parseBinary := func(
		ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind],
		left *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind],
		rbp int,
	) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		op := ctx.Consume()
		right := p.ParseExpr(ctx, rbp)

		node := ctx.Editor.NewNode(BinaryExpr)
		ctx.Editor.AttachChild(node, left)
		ctx.Editor.AttachChild(node, right)
		ctx.Editor.AddToken(node, op)

		return node
	}

	p.RegisterInfix(PlusTok, 10, pratt.Left, parseBinary)
	p.RegisterInfix(StarTok, 20, pratt.Left, parseBinary)

	p.RegisterPostfix(LParenTok, 30, func(
		ctx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind],
		left *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind],
	) *syntaxa.SyntaxaASTNode[rune, Token, TokenRole, NodeKind] {

		ctx.Consume()
		ctx.Match(RParenTok)

		node := ctx.Editor.NewNode(CallExpr)
		ctx.Editor.AttachChild(node, left)
		return node
	})

	// ---------- Grammar ----------

	selector := func(
		selectCtx syntaxa.SelectRuleContext[rune, Token, TokenRole],
	) syntaxa.ParserRule[rune, Token, TokenRole, LexerState, NodeKind] {

		// ---------------- Expression ----------------

		expr := func(
			execCtx syntaxa.ExecRuleContext[rune, Token, TokenRole, LexerState, NodeKind],
		) (syntaxa.RuleResult[rune, Token, TokenRole, NodeKind], bool) {

			execCtx.PushSkipRoles(TriviaRole)
			defer execCtx.PopSkipRoles()

			n := p.ParseExpr(execCtx, 0)
			if n == nil {
				return rd.NoNode[rune, Token, TokenRole, NodeKind](), false
			}

			return rd.NodeResult[rune, Token, TokenRole, NodeKind](n), true
		}

		// ---------------- Statement ----------------

		stmt := rd.TopLevel(
			rd.SequenceAs(
				StmtNode,
				expr,
				rd.Tok[rune, Token, TokenRole, LexerState, NodeKind](SemicolonTok),
			),
		)

		return stmt
	}

	return LangSpecCreate(
		lexer,
		ParserSpecCreate(RootNode, ErrorNode, selector),
	)
}

// ============================================================
// End-to-end test
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
// AST validation (SAFE ACCESS)
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
				return "syntax errors: <nil>"
			}
			return fmt.Sprintf("syntax errors: %+v", errs.Errors)
		}(),
		"no syntax errors",
		t,
	)

	children := root.Children()

	ftesting.Assert(
		len(children) == 1,
		"expected single statement",
		"single statement ok",
		t,
	)

	stmt := children[0]
	stmtChildren := stmt.Children()

	expr := stmtChildren[0]

	ftesting.Assert(
		expr.Kind() == BinaryExpr,
		"expected + at root",
		"binary root ok",
		t,
	)

	exprChildren := expr.Children()

	left := exprChildren[0]
	right := exprChildren[1]

	ftesting.Assert(
		left.Kind() == CallExpr,
		"expected call on left",
		"call ok",
		t,
	)

	ftesting.Assert(
		right.Kind() == BinaryExpr,
		"expected multiplication on right",
		"precedence ok",
		t,
	)
}
