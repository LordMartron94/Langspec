package generator

/*
Fingerprint-based template mining for generated .lspec output.

Goals (see LangSpec templates plan): find repeated structural subtrees in the lowered
grammar IR, emit synthetic template declarations, and replace matching sites with
template calls to shrink emitted text.

Tuning defaults are conservative so we do not explode output or emit fragile templates.
*/

import (
	"cmp"
	"fmt"
	"lexarch"
	"slices"
	"strconv"
	"strings"
	"syntaxa"
)

const (
	// minFingerprintOccurrences is the minimum number of identical-shape subtrees required
	// before promoting a synthetic template.
	minFingerprintOccurrences = 2
	// minFingerprintNodes excludes tiny fragments (single token, etc.).
	minFingerprintNodes = 4
	// maxGeneratedTemplates caps how many synthetic templates we emit per file.
	maxGeneratedTemplates = 12
	// maxTemplateParams limits arity of generated templates.
	maxTemplateParams = 8
)

// genTemplatePlan carries mined templates and replacement roots for the parse decompiler.
type genTemplatePlan[TNodeKind comparable] struct {
	// replaceRoot maps a grammar subtree root pointer to replacement metadata.
	replaceRoot map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*genTemplateSite[TNodeKind]
	// templatesBlock is nil when no templates were mined; otherwise concatenated declarations.
	templatesBlock Doc
}

type genTemplateSite[TNodeKind comparable] struct {
	name string
	// proto is the canonical subtree shape (first occurrence).
	proto *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]
	// inst is this occurrence root (structurally matches proto).
	inst *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]
}

func buildGenTemplatePlan[TNodeKind comparable](
	pkg *syntaxa.GrammarPackage[TNodeKind],
) *genTemplatePlan[TNodeKind] {
	plan := &genTemplatePlan[TNodeKind]{
		replaceRoot: make(map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]*genTemplateSite[TNodeKind]),
	}

	type candidate struct {
		shape   string
		proto   *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]
		sites   []*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]
		nodes   int
		benefit int
	}

	shapes := make(map[string][]*syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	for _, label := range pkg.SortedGrammarLabels {
		root := pkg.Grammars[label]
		for _, n := range enumerateGrammarNodes(root) {
			if n.IsContextBoundary {
				continue
			}
			shape, ok := grammarFingerprintShape(n)
			if !ok {
				continue
			}
			if grammarSubtreeNodeCount(n) < minFingerprintNodes {
				continue
			}
			shapes[shape] = append(shapes[shape], n)
		}
	}

	var cands []candidate
	for shape, sites := range shapes {
		if len(sites) < minFingerprintOccurrences {
			continue
		}
		slices.SortFunc(sites, func(a, b *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) int {
			return cmp.Compare(fmt.Sprintf("%p", a), fmt.Sprintf("%p", b))
		})
		proto := sites[0]
		okAll := true
		for _, s := range sites[1:] {
			if !grammarStructMatch(proto, s) {
				okAll = false
				break
			}
		}
		if !okAll {
			continue
		}
		names, _ := collectTemplateParams(proto)
		if len(names) == 0 || len(names) > maxTemplateParams {
			continue
		}
		nc := grammarSubtreeNodeCount(proto)
		benefit := (len(sites) - 1) * nc
		cands = append(cands, candidate{
			shape: shape, proto: proto, sites: sites, nodes: nc, benefit: benefit,
		})
	}

	slices.SortFunc(cands, func(a, b candidate) int {
		if a.benefit != b.benefit {
			return b.benefit - a.benefit
		}
		return strings.Compare(a.shape, b.shape)
	})

	claimed := make(map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]bool)
	tidx := 0
	var declPieces []Doc
	for _, c := range cands {
		if tidx >= maxGeneratedTemplates {
			break
		}
		var picked []*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]
		for _, root := range c.sites {
			if subtreeTouchesClaimed(root, claimed) {
				continue
			}
			picked = append(picked, root)
			markSubtreeClaimed(root, claimed)
		}
		if len(picked) < minFingerprintOccurrences {
			unmarkSubtrees(picked, claimed)
			continue
		}
		name := fmt.Sprintf("GenTempl_%d", tidx)
		tidx++
		names, types := collectTemplateParams(c.proto)
		decl := buildSyntheticTemplateDeclDoc(name, names, types, c.proto)
		if len(declPieces) > 0 {
			declPieces = append(declPieces, line())
		}
		declPieces = append(declPieces, decl)
		for _, root := range picked {
			plan.replaceRoot[root] = &genTemplateSite[TNodeKind]{
				name: name, proto: c.proto, inst: root,
			}
		}
	}

	if len(declPieces) > 0 {
		plan.templatesBlock = concat(declPieces...)
	}
	return plan
}

func unmarkSubtrees[TNodeKind comparable](roots []*syntaxa.Grammar[lexarch.TokenKind, TNodeKind], claimed map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]bool) {
	for _, r := range roots {
		for _, n := range enumerateGrammarNodes(r) {
			delete(claimed, n)
		}
	}
}

func enumerateGrammarNodes[TNodeKind comparable](root *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) []*syntaxa.Grammar[lexarch.TokenKind, TNodeKind] {
	if root == nil {
		return nil
	}
	var out []*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]
	var walk func(*syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if g == nil {
			return
		}
		out = append(out, g)
		for _, ch := range g.Children {
			walk(ch)
		}
	}
	walk(root)
	return out
}

func grammarSubtreeNodeCount[TNodeKind comparable](root *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) int {
	return len(enumerateGrammarNodes(root))
}

func subtreeTouchesClaimed[TNodeKind comparable](root *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], claimed map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]bool) bool {
	for _, n := range enumerateGrammarNodes(root) {
		if claimed[n] {
			return true
		}
	}
	return false
}

func markSubtreeClaimed[TNodeKind comparable](root *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], claimed map[*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]]bool) {
	for _, n := range enumerateGrammarNodes(root) {
		claimed[n] = true
	}
}

/*
grammarFingerprintShape returns a structural signature with token/rule/output shape
collapsed to holes (V=virtual token, E=emit node+token, R=rule ref). Returns ok=false
for subtrees we do not mine (nest, choice, predict, epsilon, etc.).
*/
func grammarFingerprintShape[TNodeKind comparable](g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) (string, bool) {
	if g == nil || len(g.Lookaheads) > 0 || g.IsContextBoundary {
		return "", false
	}
	switch g.Kind {
	case syntaxa.GEpsilon:
		return "", false
	case syntaxa.GToken:
		if g.OutputNodeKind == nil {
			return "V", true
		}
		return "E", true
	case syntaxa.GReference:
		return "R", true
	case syntaxa.GConcat:
		if len(g.Children) == 0 {
			return "", false
		}
		var parts []string
		for _, ch := range g.Children {
			s, ok := grammarFingerprintShape(ch)
			if !ok {
				return "", false
			}
			parts = append(parts, s)
		}
		return "K(" + strings.Join(parts, ",") + ")", true
	case syntaxa.GRepeat:
		if len(g.Children) != 1 {
			return "", false
		}
		inner, ok := grammarFingerprintShape(g.Children[0])
		if !ok {
			return "", false
		}
		maxStr := "*"
		if g.Max != nil {
			maxStr = strconv.Itoa(*g.Max)
		}
		return fmt.Sprintf("Rep(%d,%s,%s)", g.Min, maxStr, inner), true
	case syntaxa.GOptional:
		if len(g.Children) != 1 {
			return "", false
		}
		inner, ok := grammarFingerprintShape(g.Children[0])
		if !ok {
			return "", false
		}
		return "O(" + inner + ")", true
	case syntaxa.GChoice, syntaxa.GNest:
		return "", false
	default:
		return "", false
	}
}

func grammarStructMatch[TNodeKind comparable](a, b *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	if len(a.Lookaheads) != len(b.Lookaheads) {
		return false
	}
	switch a.Kind {
	case syntaxa.GToken:
		if (a.OutputNodeKind == nil) != (b.OutputNodeKind == nil) {
			return false
		}
		return true
	case syntaxa.GReference:
		return true
	case syntaxa.GConcat:
		if len(a.Children) != len(b.Children) {
			return false
		}
		for i := range a.Children {
			if !grammarStructMatch(a.Children[i], b.Children[i]) {
				return false
			}
		}
		return true
	case syntaxa.GRepeat:
		if a.Min != b.Min {
			return false
		}
		if (a.Max == nil) != (b.Max == nil) {
			return false
		}
		if a.Max != nil && b.Max != nil && *a.Max != *b.Max {
			return false
		}
		return grammarStructMatch(a.Children[0], b.Children[0])
	case syntaxa.GOptional:
		return grammarStructMatch(a.Children[0], b.Children[0])
	default:
		return false
	}
}

func collectTemplateParams[TNodeKind comparable](proto *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) (names []string, types []string) {
	idx := 0
	var walk func(*syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if g == nil {
			return
		}
		switch g.Kind {
		case syntaxa.GToken:
			if g.OutputNodeKind == nil {
				names = append(names, fmt.Sprintf("$g%d", idx))
				types = append(types, "Token")
				idx++
				return
			}
			names = append(names, fmt.Sprintf("$g%d", idx), fmt.Sprintf("$g%d", idx+1))
			types = append(types, "Node", "Token")
			idx += 2
		case syntaxa.GReference:
			names = append(names, fmt.Sprintf("$g%d", idx))
			types = append(types, "Rule")
			idx++
		case syntaxa.GConcat:
			for _, ch := range g.Children {
				walk(ch)
			}
		case syntaxa.GRepeat, syntaxa.GOptional:
			walk(g.Children[0])
		}
	}
	walk(proto)
	return names, types
}

func buildSyntheticTemplateDeclDoc[TNodeKind comparable](
	tplName string,
	paramNames []string,
	paramTypes []string,
	proto *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) Doc {
	var sigParts []Doc
	for i := range paramNames {
		if i > 0 {
			sigParts = append(sigParts, doctext(", "))
		}
		sigParts = append(sigParts,
			doctext(paramNames[i]),
			doctext(" : "),
			doctext(paramTypes[i]),
		)
	}
	sig := concat(sigParts...)
	body := emitTemplateBodyFromProto(proto, paramNames)
	return concat(
		doctext("template "),
		doctext(tplName),
		doctext("("),
		sig,
		doctext(") {"),
		nest(1, concat(line(), body)),
		line(),
		doctext("}"),
	)
}

func emitTemplateBodyFromProto[TNodeKind comparable](proto *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], params []string) Doc {
	i := 0
	var walk func(*syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) Doc
	walk = func(g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) Doc {
		if g == nil {
			return doctext("")
		}
		switch g.Kind {
		case syntaxa.GToken:
			if g.OutputNodeKind == nil {
				p := params[i]
				i++
				return concat(doctext("virtual "), doctext(p))
			}
			n := params[i]
			t := params[i+1]
			i += 2
			return concat(doctext(n), doctext(" : "), doctext(t))
		case syntaxa.GReference:
			p := params[i]
			i++
			return doctext(p)
		case syntaxa.GConcat:
			var docs []Doc
			force := len(g.Children) >= 1
			for j, ch := range g.Children {
				if j > 0 {
					docs = append(docs, line())
				}
				cd := walk(ch)
				if ch.Kind == syntaxa.GChoice {
					force = true
				}
				docs = append(docs, cd)
			}
			if force {
				return concat(docs...)
			}
			return group(concat(docs...))
		case syntaxa.GRepeat:
			inner := wrapTemplateBodyIfComplex(g.Children[0], walk(g.Children[0]))
			mod := repeatModifierString(g.Min, g.Max)
			return concat(inner, doctext(mod))
		case syntaxa.GOptional:
			inner := wrapTemplateBodyIfComplex(g.Children[0], walk(g.Children[0]))
			return concat(inner, doctext("?"))
		default:
			return doctext("")
		}
	}
	return walk(proto)
}

func wrapTemplateBodyIfComplex[TNodeKind comparable](g *syntaxa.Grammar[lexarch.TokenKind, TNodeKind], body Doc) Doc {
	isComplex := g.Kind == syntaxa.GChoice || g.Kind == syntaxa.GConcat || g.Kind == syntaxa.GRepeat || g.Kind == syntaxa.GOptional
	if !isComplex {
		return body
	}
	return concat(doctext("("), nest(1, concat(line(), body)), line(), doctext(")"))
}

func repeatModifierString(min int, max *int) string {
	if min == 0 && max == nil {
		return "*"
	}
	if min == 1 && max == nil {
		return "+"
	}
	maxStr := ""
	if max != nil {
		maxStr = strconv.Itoa(*max)
	}
	return fmt.Sprintf("{%d,%s}", min, maxStr)
}

func (d *parseDecompiler[TNodeKind]) emitFingerprintTemplateCall(site *genTemplateSite[TNodeKind]) Doc {
	args := parallelTemplateArgDocs(d, site.proto, site.inst)
	var argDocs []Doc
	for i, a := range args {
		if i > 0 {
			argDocs = append(argDocs, doctext(", "))
		}
		argDocs = append(argDocs, a)
	}
	return concat(
		doctext("call "),
		doctext(d.sanitizer(site.name)),
		doctext("("),
		concat(argDocs...),
		doctext(")"),
	)
}

func parallelTemplateArgDocs[TNodeKind comparable](
	d *parseDecompiler[TNodeKind],
	proto, inst *syntaxa.Grammar[lexarch.TokenKind, TNodeKind],
) []Doc {
	var out []Doc
	var walk func(a, b *syntaxa.Grammar[lexarch.TokenKind, TNodeKind])
	walk = func(a, b *syntaxa.Grammar[lexarch.TokenKind, TNodeKind]) {
		if a == nil || b == nil {
			return
		}
		switch a.Kind {
		case syntaxa.GToken:
			if a.OutputNodeKind == nil {
				out = append(out, doctext(d.tokenFormatter(b.Token)))
				return
			}
			out = append(out, doctext(d.sanitizer(d.nodeKindFormatter(*b.OutputNodeKind))))
			out = append(out, doctext(d.tokenFormatter(b.Token)))
		case syntaxa.GReference:
			out = append(out, doctext(d.sanitizer(string(b.ReferenceTarget))))
		case syntaxa.GConcat:
			for i := range a.Children {
				walk(a.Children[i], b.Children[i])
			}
		case syntaxa.GRepeat, syntaxa.GOptional:
			walk(a.Children[0], b.Children[0])
		}
	}
	walk(proto, inst)
	return out
}
