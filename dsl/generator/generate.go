package generator

import (
	"autarch/pattern"
	"cmp"
	"fmt"
	"foundation/system"
	"foundation/text"
	"lexarch"
	"slices"
	"strings"
	"syntaxa"
	"unicode"
)

// ----------------------------------------------------------------- TYPES

type GeneratorConfig[TToken, TTokenRole, TNodeKind comparable] struct {
	targetLSpecVersion string

	tokenFormatter     func(token TToken) string
	tokenRoleFormatter func(tokenRole TTokenRole) string
	nodeKindFormatter  func(kind TNodeKind) string

	enableSublime bool
	emitRegex     bool

	baseScopes map[TToken]string

	ignoredRoles []TTokenRole

	columnThreshold int

	eofToken *TToken
}

func GeneratorConfigCreate[TToken, TTokenRole, TNodeKind comparable](
	targetLSpecVersion string,
	skippedRoles []TTokenRole,
) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	return &GeneratorConfig[TToken, TTokenRole, TNodeKind]{
		targetLSpecVersion: targetLSpecVersion,
		tokenFormatter: func(token TToken) string {
			return fmt.Sprintf("%v", token)
		},
		tokenRoleFormatter: func(tokenRole TTokenRole) string {
			return fmt.Sprintf("%v", tokenRole)
		},
		nodeKindFormatter: func(kind TNodeKind) string {
			return fmt.Sprintf("%v", kind)
		},
		baseScopes:      make(map[TToken]string),
		columnThreshold: 80,
		ignoredRoles:    skippedRoles,
		eofToken:        nil,
	}
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithEnableSublime(value bool) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.enableSublime = value
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithEmitRegEx(value bool) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.emitRegex = value
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithBaseScopes(baseScopes map[TToken]string) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.baseScopes = baseScopes
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithTokenFormatter(formatter func(token TToken) string) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.tokenFormatter = formatter
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithTokenRoleFormatter(formatter func(tokenRole TTokenRole) string) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.tokenRoleFormatter = formatter
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithNodeKindFormatter(formatter func(kind TNodeKind) string) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.nodeKindFormatter = formatter
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithColumnThresholdHint(amount int) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.columnThreshold = amount
	return g
}

func (g *GeneratorConfig[TToken, TTokenRole, TNodeKind]) WithEofToken(token TToken) *GeneratorConfig[TToken, TTokenRole, TNodeKind] {
	g.eofToken = &token
	return g
}

// ----------------------------------------------------------------- ENTRY

func GenerateLSpec[TToken, TTokenRole, TNodeKind, TLexerState comparable](
	grammarPackage *syntaxa.GrammarPackage[rune, TToken, TTokenRole, TNodeKind, TLexerState],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	outputPath string,
	configuration *GeneratorConfig[TToken, TTokenRole, TNodeKind],
) error {
	if !system.PathHasExt(outputPath, ".lspec") {
		return fmt.Errorf("given path '%s' is not an lspec path", outputPath)
	}

	gen := createGenerator(grammarPackage, lexingRuleSet, configuration)
	document := gen.buildDocument()

	printer := newPrettyPrinter(configuration.columnThreshold)
	renderedOutput := printer.render(document)

	if err := system.FileWriteString(outputPath, renderedOutput); err != nil {
		return fmt.Errorf("could not write output file: %w", err)
	}

	return nil
}

func createGenerator[TToken, TTokenRole, TNodeKind, TLexerState comparable](
	grammarPackage *syntaxa.GrammarPackage[rune, TToken, TTokenRole, TNodeKind, TLexerState],
	lexingRuleSet *lexarch.LexingRuleset[rune, TToken, TTokenRole],
	configuration *GeneratorConfig[TToken, TTokenRole, TNodeKind],
) *generator[TToken, TTokenRole, TNodeKind, TLexerState] {
	return &generator[TToken, TTokenRole, TNodeKind, TLexerState]{
		grammarPackage:     grammarPackage,
		lexingRuleSet:      lexingRuleSet,
		targetLSpecVersion: configuration.targetLSpecVersion,
		enableSublime:      configuration.enableSublime,
		tokenFormatter:     configuration.tokenFormatter,
		tokenRoleFormatter: configuration.tokenRoleFormatter,
		nodeKindFormatter:  configuration.nodeKindFormatter,
		emitRegex:          configuration.emitRegex,
		baseScopes:         configuration.baseScopes,
		columnThreshold:    configuration.columnThreshold,
		ignoredTokenRoles:  configuration.ignoredRoles,
		eofToken:           configuration.eofToken,
	}
}

// ----------------------------------------------------------------- INTERNAL HELPERS

type generator[TToken, TTokenRole, TNodeKind, TLexerState comparable] struct {
	grammarPackage *syntaxa.GrammarPackage[rune, TToken, TTokenRole, TNodeKind, TLexerState]
	lexingRuleSet  *lexarch.LexingRuleset[rune, TToken, TTokenRole]

	targetLSpecVersion string
	enableSublime      bool
	emitRegex          bool

	tokenFormatter     func(token TToken) string
	tokenRoleFormatter func(tokenRole TTokenRole) string
	nodeKindFormatter  func(kind TNodeKind) string

	baseScopes map[TToken]string

	ignoredTokenRoles []TTokenRole
	eofToken          *TToken

	columnThreshold int
}

// --- SECTIONS

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildDocument() Doc {
	return concat(
		g.buildHeader(), line(), line(),
		g.buildPragmaSection(), line(), line(),
		g.buildPatternSection(), line(), line(),
		g.buildLexSection(), line(), line(),
		g.buildPrattSection(), line(), line(),
		g.buildParseSection(), line(),
	)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildHeader() Doc {
	headerStr := fmt.Sprintf("--- \"%s\" %s | lspec %s ---",
		g.grammarPackage.Name,
		g.formatVersion(g.grammarPackage.Version),
		g.formatVersion(g.targetLSpecVersion))
	return doctext(headerStr)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildPragmaSection() Doc {
	var bodyDocs []Doc
	if g.enableSublime {
		bodyDocs = append(bodyDocs, g.buildSublimePragma())
	}

	if len(bodyDocs) == 0 {
		return concat(doctext("PRAGMA {"), line(), doctext("}"))
	}

	return concat(
		doctext("PRAGMA {"),
		nest(1, concat(line(), concat(bodyDocs...))),
		line(),
		doctext("}"),
	)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildSublimePragma() Doc {
	return concat(
		doctext("tool.sublime {"),
		nest(1, concat(
			line(), doctext("enable = true;"),
			line(), doctext(`path = "<insert path here>";`),
			line(), doctext("auto-generate = true;"),
		)),
		line(), doctext("}"),
	)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildPatternSection() Doc {
	rules := g.lexingRuleSet.GetRules()
	if len(rules) == 0 {
		return concat(doctext("PATTERN {"), line(), doctext("}"))
	}

	var bodyDocs []Doc
	for i, lexRule := range rules {
		if i > 0 {
			bodyDocs = append(bodyDocs, line())
		}
		bodyDocs = append(bodyDocs, g.buildPatternRule(lexRule))
	}

	return concat(
		doctext("PATTERN {"),
		nest(1, concat(line(), concat(bodyDocs...))),
		line(),
		doctext("}"),
	)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildPatternRule(lexRule lexarch.LexerRuleReadOnly[rune, TToken, TTokenRole]) Doc {
	if g.emitRegex {
		return g.buildRegExRuleDoc(lexRule)
	}
	return g.buildDecompiledRuleDoc(lexRule)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildRegExRuleDoc(lexRule lexarch.LexerRuleReadOnly[rune, TToken, TTokenRole]) Doc {
	regex, err := lexRule.Pattern.ToRegEx()
	if err != nil {
		panic(fmt.Errorf("engine error while converting pattern to RegEx: %w", err))
	}
	ruleStr := fmt.Sprintf("%s : `%s`;", g.formatPatternName(lexRule.Token), regex)
	return doctext(ruleStr)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildDecompiledRuleDoc(lexRule lexarch.LexerRuleReadOnly[rune, TToken, TTokenRole]) Doc {
	patternName := g.formatPatternName(lexRule.Token)
	decompiler := newLSpecDecompiler()
	lexRule.Pattern.Accept(decompiler.visitor())

	ruleStr := fmt.Sprintf("%s : %s;", patternName, decompiler.String())

	return doctext(ruleStr)
}

type lspecDecompiler struct {
	out *strings.Builder
}

func newLSpecDecompiler() *lspecDecompiler {
	return &lspecDecompiler{
		out: &strings.Builder{},
	}
}

func (d *lspecDecompiler) visitor() pattern.RegulaVisitor[rune] {
	var v pattern.RegulaVisitor[rune]
	v = pattern.RegulaVisitor[rune]{
		VisitLiteral: d.visitLiteral,
		VisitClass:   d.visitClass,
		VisitConcat:  func(l, r *pattern.RegulaAST[rune]) { d.visitConcat(v, l, r) },
		VisitUnion:   func(l, r *pattern.RegulaAST[rune]) { d.visitUnion(v, l, r) },
		VisitRepeat:  func(sub *pattern.RegulaAST[rune], min, max int) { d.visitRepeat(v, sub, min, max) },
		VisitCapture: func(sub *pattern.RegulaAST[rune]) { panic("capture not supported in LSpec") },
	}
	return v
}

func (d *lspecDecompiler) visitLiteral(values []rune) {
	d.out.WriteString(`"`)
	d.out.WriteString(text.GoStringEscaper.Escape(string(values)))
	d.out.WriteString(`"`)
}

func (d *lspecDecompiler) visitClass(ranges, negatedFrom []pattern.CharRange[rune]) {
	targetRanges := ranges
	if len(negatedFrom) > 0 {
		d.out.WriteString("!")
		targetRanges = negatedFrom
	}

	if len(targetRanges) == 1 {
		d.writeSingleRange(targetRanges[0])
		return
	}

	d.out.WriteString("[")
	for i, r := range targetRanges {
		if i > 0 {
			d.out.WriteString(", ")
		}
		d.writeSingleRange(r)
	}
	d.out.WriteString("]")
}

func (d *lspecDecompiler) writeSingleRange(r pattern.CharRange[rune]) {
	loEscaped := text.GoCharEscaper.EscapeRune(r.Lo)
	if r.Lo == r.Hi {
		d.out.WriteString(fmt.Sprintf("'%s'", loEscaped))
		return
	}

	hiEscaped := text.GoCharEscaper.EscapeRune(r.Hi)
	d.out.WriteString(fmt.Sprintf("'%s'..'%s'", loEscaped, hiEscaped))
}

func (d *lspecDecompiler) visitConcat(v pattern.RegulaVisitor[rune], left, right *pattern.RegulaAST[rune]) {
	left.Accept(v)
	d.out.WriteString(" ")
	right.Accept(v)
}

func (d *lspecDecompiler) visitUnion(v pattern.RegulaVisitor[rune], left, right *pattern.RegulaAST[rune]) {
	d.out.WriteString("(")
	left.Accept(v)
	d.out.WriteString(" | ")
	right.Accept(v)
	d.out.WriteString(")")
}

func (d *lspecDecompiler) visitRepeat(v pattern.RegulaVisitor[rune], sub *pattern.RegulaAST[rune], min, max int) {
	d.out.WriteString("(")
	sub.Accept(v)
	d.out.WriteString(")")

	if min == 0 && max == -1 {
		d.out.WriteString("*")
		return
	}
	if min == 1 && max == -1 {
		d.out.WriteString("+")
		return
	}
	if min == 0 && max == 1 {
		d.out.WriteString("?")
		return
	}
	d.writeBoundedRepeat(min, max)
}

func (d *lspecDecompiler) writeBoundedRepeat(min, max int) {
	if max == -1 {
		d.out.WriteString(fmt.Sprintf("{%d,}", min))
		return
	}
	d.out.WriteString(fmt.Sprintf("{%d,%d}", min, max))
}

func (d *lspecDecompiler) String() string {
	return d.out.String()
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildLexSection() Doc {
	rules := g.lexingRuleSet.GetRules()
	if len(rules) == 0 {
		return concat(doctext("LEX {"), line(), doctext("}"))
	}

	rows := g.buildLexRows(rules)
	g.sortLexRows(rows)
	maxTok, maxRole, maxPat := g.calculateLexColumnWidths(rows)

	bodyDoc := g.buildLexRowsBlockDoc(rows, maxTok, maxRole, maxPat)

	return concat(
		doctext("LEX {"),
		nest(1, concat(line(), bodyDoc)),
		line(),
		doctext("}"),
	)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildLexRowsBlockDoc(rows []lexRow, maxTok, maxRole, maxPat int) Doc {
	var docs []Doc
	lastPriority := -1
	if len(rows) > 0 {
		lastPriority = rows[0].Priority
	}

	for i, r := range rows {
		if i > 0 && r.Priority != lastPriority {
			docs = append(docs, line(), line())
			lastPriority = r.Priority
		} else if i > 0 {
			docs = append(docs, line())
		}
		docs = append(docs, g.buildAlignedLexRowDoc(r, maxTok, maxRole, maxPat))
	}

	return concat(docs...)
}

type lexRow struct {
	Priority int
	Token    string
	Role     string
	Pattern  string
	Meta     string
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildLexRows(rules []lexarch.LexerRuleReadOnly[rune, TToken, TTokenRole]) []lexRow {
	rows := make([]lexRow, 0, len(rules))
	for _, rule := range rules {
		rows = append(rows, g.buildSingleLexRow(rule))
	}
	return rows
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildSingleLexRow(rule lexarch.LexerRuleReadOnly[rune, TToken, TTokenRole]) lexRow {
	var attributes []string

	if baseScope, ok := g.baseScopes[rule.Token]; ok {
		attributes = append(attributes, fmt.Sprintf("scope=\"%s\"", baseScope))
	}
	if g.eofToken != nil && rule.Token == *g.eofToken {
		attributes = append(attributes, "EOF=true")
	}

	meta := ""
	if len(attributes) > 0 {
		meta = fmt.Sprintf("%%%% %s %%%%", strings.Join(attributes, " "))
	}

	return lexRow{
		Priority: rule.Priority,
		Token:    g.tokenFormatter(rule.Token),
		Role:     g.tokenRoleFormatter(rule.Role),
		Pattern:  g.formatPatternName(rule.Token),
		Meta:     meta,
	}
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) sortLexRows(rows []lexRow) {
	slices.SortFunc(rows, func(a, b lexRow) int {
		if a.Priority != b.Priority {
			return cmp.Compare(b.Priority, a.Priority)
		}
		return strings.Compare(a.Token, b.Token)
	})
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) calculateLexColumnWidths(rows []lexRow) (int, int, int) {
	maxTok, maxRole, maxPat := 0, 0, 0
	for _, r := range rows {
		if len(r.Token) > maxTok {
			maxTok = len(r.Token)
		}
		if len(r.Role) > maxRole {
			maxRole = len(r.Role)
		}
		if len(r.Pattern) > maxPat {
			maxPat = len(r.Pattern)
		}
	}
	return maxTok, maxRole, maxPat
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildAlignedLexRowDoc(r lexRow, maxTok, maxRole, maxPat int) Doc {
	format := fmt.Sprintf("%%d %%-%ds -> %%-%ds : %%-%ds", maxTok, maxRole, maxPat)
	base := fmt.Sprintf(format, r.Priority, r.Token, r.Role, r.Pattern)

	if r.Meta != "" {
		base += " " + r.Meta
	}
	base += ";"
	return doctext(base)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildPrattSection() Doc {
	return concat(
		doctext("PRATT {"),
		nest(1, concat(
			line(), doctext("// In an automatically generated .lspec file, pratt is not present."),
			line(), doctext("// Namely, it has been \"resolved\"/\"flattened\" into normal parsing rules."),
		)),
		line(),
		doctext("}"),
	)
}

// --- PARSE SECTION REWRITE ---

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildParseSection() Doc {
	rules := g.grammarPackage.Grammars
	if len(rules) == 0 {
		return concat(doctext("PARSE {"), line(), doctext("}"))
	}

	var sectionDocs []Doc

	ignoredTokens := g.ignoredTokenRoles
	if len(ignoredTokens) > 0 {
		var tokenDocs []Doc
		for i, t := range ignoredTokens {
			if i > 0 {
				tokenDocs = append(tokenDocs, space())
			}
			tokenDocs = append(tokenDocs, doctext(g.tokenRoleFormatter(t)))
		}

		ignoreDoc := concat(
			doctext("IGNORE {"),
			nest(1, concat(line(), group(concat(tokenDocs...)))),
			line(),
			doctext("};"),
		)
		sectionDocs = append(sectionDocs, ignoreDoc, line(), line())
	}

	// 2. Build Standard Rules
	labels := g.grammarPackage.SortedGrammarLabels
	for i, label := range labels {
		labelStr := string(label)
		if i > 0 {
			sectionDocs = append(sectionDocs, line(), line())
		}
		sectionDocs = append(sectionDocs, g.buildParseRuleDoc(labelStr, rules[label]))
	}

	return concat(
		doctext("PARSE {"),
		nest(1, concat(line(), concat(sectionDocs...))),
		line(),
		doctext("}"),
	)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) buildParseRuleDoc(labelStr string, ruleNode *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	decompiler := newParseDecompiler(g.tokenFormatter, g.nodeKindFormatter, g.sanitizeIdentifier, g.columnThreshold)
	return decompiler.BuildRuleDoc(labelStr, ruleNode)
}

type layoutMode int

const (
	layoutInline layoutMode = iota
	layoutBlock
)

type parseDecompiler[TToken, TNodeKind comparable] struct {
	tokenFormatter    func(TToken) string
	nodeKindFormatter func(TNodeKind) string
	sanitizer         func(string) string
	columnThreshold   int
	currentRuleLabel  string
}

func newParseDecompiler[TToken, TNodeKind comparable](
	tokenFormatter func(TToken) string,
	nodeKindFormatter func(TNodeKind) string,
	sanitizer func(string) string,
	columnThreshold int,
) *parseDecompiler[TToken, TNodeKind] {
	return &parseDecompiler[TToken, TNodeKind]{
		tokenFormatter:    tokenFormatter,
		nodeKindFormatter: nodeKindFormatter,
		sanitizer:         sanitizer,
		columnThreshold:   columnThreshold,
	}
}

func (d *parseDecompiler[TToken, TNodeKind]) BuildRuleDoc(labelStr string, ruleNode *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	d.currentRuleLabel = labelStr

	headerDoc := d.buildHeaderDoc(labelStr, ruleNode)
	bodyDoc := d.walk(ruleNode)

	return concat(
		headerDoc,
		doctext(" {"),
		nest(1, concat(line(), bodyDoc)),
		line(),
		doctext("};"),
	)
}

func (d *parseDecompiler[TToken, TNodeKind]) buildHeaderDoc(labelStr string, ruleNode *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	isTransparent := ruleNode.OutputNodeKind == nil || ruleNode.Kind == syntaxa.GToken || ruleNode.Kind == syntaxa.GChoice

	nodeName := labelStr
	if ruleNode.OutputNodeKind != nil {
		nodeName = fmt.Sprintf("%v", *ruleNode.OutputNodeKind)
	}

	transparencyMod := ""
	if isTransparent {
		transparencyMod = "transparent "

		nodeName = "gr_" + nodeName
	}

	base := fmt.Sprintf("%s -> %s%s", d.sanitizer(labelStr), transparencyMod, d.sanitizer(nodeName))

	if len(ruleNode.RecoveryTokens) == 0 {
		return doctext(base)
	}

	return concat(doctext(base), d.buildSyncDoc(ruleNode.RecoveryTokens))
}

func (d *parseDecompiler[TToken, TNodeKind]) buildSyncDoc(tokens []TToken) Doc {
	docs := []Doc{doctext(" sync (")}
	for i, t := range tokens {
		if i > 0 {
			docs = append(docs, space())
		}
		docs = append(docs, doctext(d.tokenFormatter(t)))
	}
	docs = append(docs, doctext(")"))
	return concat(docs...)
}

func (d *parseDecompiler[TToken, TNodeKind]) walk(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	if g == nil {
		return doctext("")
	}

	if g.IsContextBoundary && string(g.GrammarLabel) != "" && string(g.GrammarLabel) != d.currentRuleLabel {
		return doctext(d.sanitizer(string(g.GrammarLabel)))
	}

	lookaheadDoc := d.buildLookaheadDoc(g)
	nodeDoc := d.mapGrammarToDoc(g)

	if lookaheadDoc != nil {
		return concat(lookaheadDoc, nodeDoc)
	}
	return nodeDoc
}

func (d *parseDecompiler[TToken, TNodeKind]) buildLookaheadDoc(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	if len(g.Lookaheads) == 0 {
		return nil
	}

	var parts []string
	for _, la := range g.Lookaheads {
		parts = append(parts, fmt.Sprintf("%d:%s", la.Offset, d.tokenFormatter(la.Expected)))
	}

	return concat(doctext("predict ("), doctext(strings.Join(parts, ", ")), doctext(")"), space())
}

func (d *parseDecompiler[TToken, TNodeKind]) mapGrammarToDoc(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	var baseDoc Doc

	switch g.Kind {
	case syntaxa.GToken:
		baseDoc = doctext(d.tokenFormatter(g.Token))
		if g.OutputNodeKind == nil {
			baseDoc = concat(doctext("virtual "), baseDoc)
		}
	case syntaxa.GReference:
		baseDoc = doctext(d.sanitizer(string(g.ReferenceTarget)))
	case syntaxa.GEpsilon:
		baseDoc = doctext("")
	case syntaxa.GConcat:
		baseDoc = d.mapConcat(g)
	case syntaxa.GChoice:
		baseDoc = d.mapChoice(g)
	case syntaxa.GNest:
		baseDoc = d.mapNest(g)
	case syntaxa.GRepeat:
		baseDoc = d.mapRepeat(g)
	case syntaxa.GOptional:
		baseDoc = d.mapOptional(g)
	default:
		panic("unsupported grammar kind")
	}

	if g.Kind == syntaxa.GToken && g.OutputNodeKind != nil {
		nodeName := d.sanitizer(d.nodeKindFormatter(*g.OutputNodeKind))
		return concat(doctext(nodeName), doctext(" : "), baseDoc)
	}

	if g.Kind == syntaxa.GChoice && g.OutputNodeKind != nil {
		nodeName := d.sanitizer(d.nodeKindFormatter(*g.OutputNodeKind))
		return concat(
			doctext(nodeName), doctext(" : "),
			doctext("("), nest(1, concat(line(), baseDoc)), line(), doctext(")"),
		)
	}

	return baseDoc
}

func (d *parseDecompiler[TToken, TNodeKind]) mapConcat(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	var docs []Doc
	forceBreak := len(g.Children) >= 1

	for i, child := range g.Children {
		if i > 0 {
			docs = append(docs, line())
		}

		childDoc := d.concatChildDoc(g, child)

		if child.Kind == syntaxa.GChoice {
			forceBreak = true
		}

		docs = append(docs, childDoc)
	}

	if forceBreak {
		return concat(docs...)
	}

	return group(concat(docs...))
}

func (d *parseDecompiler[TToken, TNodeKind]) concatChildDoc(parent *syntaxa.Grammar[TToken, TNodeKind], child *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	childDoc := d.walk(child)

	if child.Kind == syntaxa.GChoice {
		return concat(doctext("("), nest(1, concat(line(), childDoc)), line(), doctext(")"))
	}

	return childDoc
}

func (d *parseDecompiler[TToken, TNodeKind]) mapChoice(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	var docs []Doc
	for i, child := range g.Children {
		if i > 0 {
			docs = append(docs, line(), doctext("| "))
		}
		docs = append(docs, d.choiceChildDoc(g, child))
	}
	return group(concat(docs...))
}

func (d *parseDecompiler[TToken, TNodeKind]) choiceChildDoc(parent *syntaxa.Grammar[TToken, TNodeKind], child *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	// if parent.OutputNodeKind != nil && child.Kind == syntaxa.GToken && child.OutputNodeKind == nil {
	// 	return doctext(d.tokenFormatter(child.Token))
	// }
	return d.walk(child)
}

func (d *parseDecompiler[TToken, TNodeKind]) mapNest(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	openTok := d.tokenFormatter(*g.OpenToken)
	closeTok := d.tokenFormatter(*g.CloseToken)

	// --- Short form: nest OPEN CLOSE RULE
	if refNode, ok := isReferenceBody(g); ok {
		ref := d.sanitizer(string(refNode.ReferenceTarget))

		return concat(
			doctext("nest "),
			doctext(openTok),
			space(),
			doctext(closeTok),
			space(),
			doctext(ref),
		)
	}

	// --- Normal inline form
	header := fmt.Sprintf("nest %s %s {", openTok, closeTok)

	var childDoc = doctext("")
	if len(g.Children) > 0 {
		childDoc = d.walk(g.Children[0])
	}

	return concat(
		doctext(header),
		nest(1, concat(line(), childDoc)),
		line(),
		doctext("}"),
	)
}

func isReferenceBody[TToken, TNodeKind comparable](g *syntaxa.Grammar[TToken, TNodeKind]) (*syntaxa.Grammar[TToken, TNodeKind], bool) {
	if len(g.Children) != 1 {
		return nil, false
	}

	child := g.Children[0]
	if child.Kind != syntaxa.GReference {
		return nil, false
	}

	return child, true
}

func (d *parseDecompiler[TToken, TNodeKind]) mapRepeat(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	body := d.wrapIfComplex(g.Children[0])
	modifier := d.getRepeatModifier(g.Min, g.Max)
	return concat(body, doctext(modifier))
}

func (d *parseDecompiler[TToken, TNodeKind]) mapOptional(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	body := d.wrapIfComplex(g.Children[0])
	return concat(body, doctext("?"))
}

func (d *parseDecompiler[TToken, TNodeKind]) wrapIfComplex(g *syntaxa.Grammar[TToken, TNodeKind]) Doc {
	isComplex := g.Kind == syntaxa.GChoice || g.Kind == syntaxa.GConcat || g.Kind == syntaxa.GRepeat || g.Kind == syntaxa.GOptional
	body := d.walk(g)

	if !isComplex {
		return body
	}

	return concat(
		doctext("("),
		nest(1, concat(line(), body)),
		line(),
		doctext(")"),
	)
}

func (d *parseDecompiler[TToken, TNodeKind]) getRepeatModifier(min int, max *int) string {
	if min == 0 && max == nil {
		return "*"
	}
	if min == 1 && max == nil {
		return "+"
	}

	maxStr := ""
	if max != nil {
		maxStr = fmt.Sprintf("%d", *max)
	}
	return fmt.Sprintf("{%d,%s}", min, maxStr)
}

// --- UTIL

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) formatPatternName(token TToken) string {
	raw := g.tokenFormatter(token)
	safe := g.sanitizeIdentifier(raw)
	return fmt.Sprintf("pat_%s", safe)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) formatVersion(version string) string {
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) sanitizeIdentifier(input string) string {
	if input == "" {
		return "_"
	}

	var builder strings.Builder
	for i, r := range input {
		g.appendSanitizedRune(&builder, i, r)
	}

	return g.collapseUnderscores(builder.String())
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) appendSanitizedRune(builder *strings.Builder, index int, r rune) {
	if index == 0 {
		g.appendFirstSanitizedRune(builder, r)
		return
	}
	g.appendSubsequentSanitizedRune(builder, r)
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) appendFirstSanitizedRune(builder *strings.Builder, r rune) {
	if unicode.IsLetter(r) || r == '_' {
		builder.WriteRune(r)
		return
	}
	builder.WriteByte('_')
	if unicode.IsDigit(r) {
		builder.WriteRune(r)
	}
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) appendSubsequentSanitizedRune(builder *strings.Builder, r rune) {
	if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
		builder.WriteRune(r)
		return
	}
	builder.WriteByte('_')
}

func (g *generator[TToken, TTokenRole, TNodeKind, TLexerState]) collapseUnderscores(s string) string {
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return s
}
