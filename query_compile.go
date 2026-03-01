package gotreesitter

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type queryParser struct {
	input string
	pos   int
	lang  *Language
	q     *Query
}

func (p *queryParser) parse() error {
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			break
		}

		ch := p.input[p.pos]

		if ch == '(' && p.pos+1 < len(p.input) && p.input[p.pos+1] == '#' {
			if len(p.q.patterns) == 0 {
				return fmt.Errorf("query: predicate must follow a pattern at position %d", p.pos)
			}
			pred, err := p.parsePredicate()
			if err != nil {
				return err
			}
			last := &p.q.patterns[len(p.q.patterns)-1]
			last.predicates = append(last.predicates, pred)
			if err := p.validatePatternPredicates(last); err != nil {
				return err
			}
			continue
		}

		switch {
		case ch == '(':
			// A top-level pattern.
			pat, err := p.parsePattern(0, 0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '[':
			// Top-level alternation: ["func" "return"] @keyword
			pat, err := p.parseAlternationPattern(0, 0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '"':
			// Top-level string match: "func" @keyword
			pat, err := p.parseStringPattern(0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		case isIdentStart(ch):
			// Top-level field shorthand: field: (pattern)
			pat, err := p.parseFieldShorthandPattern(0)
			if err != nil {
				return err
			}
			p.q.patterns = append(p.q.patterns, *pat)

		case ch == '.':
			return fmt.Errorf("query: unexpected top-level anchor '.' at position %d", p.pos)

		default:
			return fmt.Errorf("query: unexpected character %q at position %d", string(ch), p.pos)
		}
	}
	return nil
}

// parsePattern parses a parenthesized S-expression pattern.
// depth is the nesting depth for the steps produced.
func (p *queryParser) parsePattern(depth int, parentSymbolHint Symbol) (*Pattern, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '(' {
		return nil, fmt.Errorf("query: expected '(' at position %d", p.pos)
	}
	p.pos++ // consume '('
	p.skipWhitespaceAndComments()

	pat := &Pattern{}
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("query: unexpected end of input, expected node type or pattern")
	}

	rootIdx := -1

	// Parse the root element. This supports:
	//   - standard node patterns: (identifier ...)
	//   - parenthesized strings: ("(") @punctuation.bracket
	//   - grouping wrappers: ((identifier) ... (#set! ...))
	switch ch := p.input[p.pos]; {
	case isIdentStart(ch):
		nodeType, err := p.readIdentifier()
		if err != nil {
			return nil, fmt.Errorf("query: expected node type after '(' at position %d: %w", p.pos, err)
		}
		step, err := p.stepFromIdentifierName(depth, nodeType)
		if err != nil {
			return nil, err
		}
		pat.steps = append(pat.steps, step)
		rootIdx = 0

	case ch == '"':
		text, err := p.readString()
		if err != nil {
			return nil, err
		}
		pat.steps = append(pat.steps, QueryStep{
			captureID: -1,
			depth:     depth,
			textMatch: text,
		})
		rootIdx = 0

	case ch == '(' || ch == '[':
		innerPat, err := p.parsePatternElement(depth, parentSymbolHint)
		if err != nil {
			return nil, err
		}
		if len(innerPat.steps) == 0 {
			return nil, fmt.Errorf("query: empty grouped pattern at position %d", p.pos)
		}
		pat.steps = append(pat.steps, innerPat.steps...)
		pat.predicates = append(pat.predicates, innerPat.predicates...)
		rootIdx = 0

	default:
		return nil, fmt.Errorf("query: expected node type after '(' at position %d: query: expected identifier at position %d", p.pos, p.pos)
	}

	// Parse children, fields, and captures until ')'.
	pendingAnchor := false
	lastChildRootIdx := -1
	appendChildPattern := func(childPat *Pattern) {
		if childPat == nil || len(childPat.steps) == 0 {
			return
		}
		if pendingAnchor {
			childPat.steps[0].anchorBefore = true
			pendingAnchor = false
		}
		childRootIdx := len(pat.steps)
		pat.predicates = append(pat.predicates, childPat.predicates...)
		pat.steps = append(pat.steps, childPat.steps...)
		lastChildRootIdx = childRootIdx
	}
	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: unexpected end of input, expected ')'")
		}

		ch := p.input[p.pos]

		if ch == ')' {
			if pendingAnchor && lastChildRootIdx >= 0 {
				pat.steps[lastChildRootIdx].anchorAfter = true
			}
			p.pos++ // consume ')'
			break
		}

		if ch == '.' {
			// Anchor operators:
			//   - before child: first-child / immediate-sibling anchor
			//   - after child: last-child anchor
			// Anchors only affect child constraints at this depth.
			p.pos++
			pendingAnchor = true
			continue
		}

		if ch == '!' {
			// Field-negation constraint like !type_parameters.
			p.pos++
			p.skipWhitespaceAndComments()
			fieldName, err := p.readIdentifier()
			if err != nil {
				return nil, err
			}
			if rootIdx >= 0 && rootIdx < len(pat.steps) {
				parentSymbol := pat.steps[rootIdx].symbol
				fieldID, err := p.resolveField(fieldName, parentSymbol, parentSymbolHint)
				if err != nil {
					return nil, err
				}
				pat.steps[rootIdx].absentFields = append(pat.steps[rootIdx].absentFields, fieldID)
			}
			continue
		}

		if ch == '@' {
			// Capture for the current node.
			capName, err := p.readCapture()
			if err != nil {
				return nil, err
			}
			capID := p.ensureCapture(capName)
			if rootIdx >= 0 && rootIdx < len(pat.steps) {
				p.addCaptureToStep(&pat.steps[rootIdx], capID)
			}
			continue
		}

		if ch == '(' {
			// Predicate expression.
			if p.pos+1 < len(p.input) && p.input[p.pos+1] == '#' {
				pred, err := p.parsePredicate()
				if err != nil {
					return nil, err
				}
				pat.predicates = append(pat.predicates, pred)
				continue
			}

			// Nested pattern (child constraint).
			currentRootSymbol := Symbol(0)
			if rootIdx >= 0 && rootIdx < len(pat.steps) {
				currentRootSymbol = pat.steps[rootIdx].symbol
			}
			childPat, err := p.parsePatternElement(depth+1, currentRootSymbol)
			if err != nil {
				return nil, err
			}
			appendChildPattern(childPat)
			continue
		}

		if ch == '[' {
			// Alternation child.
			currentRootSymbol := Symbol(0)
			if rootIdx >= 0 && rootIdx < len(pat.steps) {
				currentRootSymbol = pat.steps[rootIdx].symbol
			}
			childPat, err := p.parsePatternElement(depth+1, currentRootSymbol)
			if err != nil {
				return nil, err
			}
			appendChildPattern(childPat)
			continue
		}

		if ch == '"' {
			// String child.
			currentRootSymbol := Symbol(0)
			if rootIdx >= 0 && rootIdx < len(pat.steps) {
				currentRootSymbol = pat.steps[rootIdx].symbol
			}
			childPat, err := p.parsePatternElement(depth+1, currentRootSymbol)
			if err != nil {
				return nil, err
			}
			appendChildPattern(childPat)
			continue
		}

		// Check for field: syntax (identifier followed by ':')
		if isIdentStart(ch) {
			ident, err := p.readIdentifier()
			if err != nil {
				return nil, err
			}
			afterIdent := p.pos
			p.skipWhitespaceAndComments()
			if p.pos < len(p.input) && p.input[p.pos] == ':' {
				// It's a field constraint.
				p.pos++ // consume ':'
				p.skipWhitespaceAndComments()

				parentSymbol := Symbol(0)
				if rootIdx >= 0 && rootIdx < len(pat.steps) {
					parentSymbol = pat.steps[rootIdx].symbol
				}
				fieldID, err := p.resolveField(ident, parentSymbol, parentSymbolHint)
				if err != nil {
					return nil, err
				}

				// The child pattern follows.
				if p.pos >= len(p.input) {
					return nil, fmt.Errorf("query: expected child pattern after field %q", ident)
				}

				childPat, err := p.parsePatternElement(depth+1, parentSymbol)
				if err != nil {
					return nil, err
				}
				if len(childPat.steps) > 0 {
					childPat.steps[0].field = fieldID
				}
				appendChildPattern(childPat)
			} else {
				// Bare shorthand child pattern like `_` or `identifier`.
				p.pos = afterIdent
				childPat, err := p.parseIdentifierPatternFromName(depth+1, ident)
				if err != nil {
					return nil, err
				}
				appendChildPattern(childPat)
			}
			continue
		}

		return nil, fmt.Errorf("query: unexpected character %q at position %d", string(ch), p.pos)
	}

	// Check for capture after the closing paren.
	p.skipWhitespaceAndComments()
	if quantifier, ok := p.readStepQuantifier(); ok {
		if rootIdx >= 0 && rootIdx < len(pat.steps) {
			pat.steps[rootIdx].quantifier = quantifier
		}
		p.skipWhitespaceAndComments()
	}
	for p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		capID := p.ensureCapture(capName)
		if rootIdx >= 0 && rootIdx < len(pat.steps) {
			p.addCaptureToStep(&pat.steps[rootIdx], capID)
		}
		p.skipWhitespaceAndComments()
	}

	if err := p.validatePatternPredicates(pat); err != nil {
		return nil, err
	}

	return pat, nil
}

// parseAlternationPattern parses [...] alternation syntax.
func (p *queryParser) parseAlternationPattern(depth int, parentSymbolHint Symbol) (*Pattern, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '[' {
		return nil, fmt.Errorf("query: expected '[' at position %d", p.pos)
	}
	p.pos++ // consume '['
	p.skipWhitespaceAndComments()

	var alts []alternativeSymbol

	for {
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("query: unexpected end of input in alternation")
		}

		if p.input[p.pos] == ']' {
			p.pos++ // consume ']'
			break
		}

		ch := p.input[p.pos]
		if ch == '.' {
			// Anchors inside alternations are parsed for compatibility and ignored.
			p.pos++
			continue
		}

		var branchPat *Pattern
		var err error
		if ch == '(' || ch == '[' || ch == '"' {
			branchPat, err = p.parsePatternElement(depth, parentSymbolHint)
		} else if isIdentStart(ch) {
			// Alternation may contain field shorthand branches like:
			// [name: (identifier) alias: (identifier)].
			ident, readErr := p.readIdentifier()
			if readErr != nil {
				return nil, readErr
			}
			p.skipWhitespaceAndComments()
			if p.pos < len(p.input) && p.input[p.pos] == ':' {
				p.pos++ // consume ':'
				p.skipWhitespaceAndComments()
				branchPat, err = p.parsePatternElement(depth, parentSymbolHint)
			} else {
				branchPat, err = p.parseIdentifierPatternFromName(depth, ident)
			}
		} else {
			return nil, fmt.Errorf("query: unexpected character %q in alternation at position %d", string(ch), p.pos)
		}
		if err != nil {
			return nil, err
		}
		if len(branchPat.steps) == 0 {
			continue
		}

		root := branchPat.steps[0]
		alt := alternativeSymbol{
			symbol:    root.symbol,
			isNamed:   root.isNamed,
			textMatch: root.textMatch,
			captureID: -1,
		}
		if len(branchPat.predicates) > 0 || len(branchPat.steps) > 1 {
			alt.steps = make([]QueryStep, len(branchPat.steps))
			copy(alt.steps, branchPat.steps)
			alt.predicates = make([]QueryPredicate, len(branchPat.predicates))
			copy(alt.predicates, branchPat.predicates)
		} else {
			if len(root.captureIDs) > 0 {
				for _, capID := range root.captureIDs {
					p.addCaptureToAlternative(&alt, capID)
				}
			} else if root.captureID >= 0 {
				p.addCaptureToAlternative(&alt, root.captureID)
			}
		}
		alts = append(alts, alt)
	}

	if len(alts) == 0 {
		return nil, fmt.Errorf("query: empty alternation")
	}

	step := QueryStep{
		captureID:    -1,
		depth:        depth,
		alternatives: alts,
	}

	// Check for capture after ']'.
	p.skipWhitespaceAndComments()
	if quantifier, ok := p.readStepQuantifier(); ok {
		step.quantifier = quantifier
		p.skipWhitespaceAndComments()
	}
	for p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		p.addCaptureToStep(&step, p.ensureCapture(capName))
		p.skipWhitespaceAndComments()
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

// parseStringPattern parses a "string" pattern for matching anonymous nodes.
func (p *queryParser) parseStringPattern(depth int) (*Pattern, error) {
	text, err := p.readString()
	if err != nil {
		return nil, err
	}

	step := QueryStep{
		captureID: -1,
		depth:     depth,
		textMatch: text,
	}

	// Check for capture after the string.
	p.skipWhitespaceAndComments()
	if quantifier, ok := p.readStepQuantifier(); ok {
		step.quantifier = quantifier
		p.skipWhitespaceAndComments()
	}
	for p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		p.addCaptureToStep(&step, p.ensureCapture(capName))
		p.skipWhitespaceAndComments()
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

// parsePatternElement parses one query element at the given depth.
// Supported forms:
//   - (pattern ...)
//   - [alternation ...]
//   - "string"
//   - identifier / _ (shorthand single-node pattern)
func (p *queryParser) parsePatternElement(depth int, parentSymbolHint Symbol) (*Pattern, error) {
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("query: expected pattern element at end of input")
	}

	switch ch := p.input[p.pos]; {
	case ch == '(':
		return p.parsePattern(depth, parentSymbolHint)
	case ch == '[':
		return p.parseAlternationPattern(depth, parentSymbolHint)
	case ch == '"':
		return p.parseStringPattern(depth)
	case isIdentStart(ch):
		name, err := p.readIdentifier()
		if err != nil {
			return nil, err
		}
		return p.parseIdentifierPatternFromName(depth, name)
	default:
		return nil, fmt.Errorf("query: expected '(' or '[' or '\"' or identifier at position %d", p.pos)
	}
}

func (p *queryParser) stepFromIdentifierName(depth int, name string) (QueryStep, error) {
	sym, isNamed, err := p.resolveSymbol(name)
	if err != nil {
		return QueryStep{}, err
	}

	return QueryStep{
		symbol:    sym,
		isNamed:   isNamed,
		captureID: -1,
		depth:     depth,
	}, nil
}

func (p *queryParser) parseIdentifierPatternFromName(depth int, name string) (*Pattern, error) {
	step, err := p.stepFromIdentifierName(depth, name)
	if err != nil {
		return nil, err
	}

	p.skipWhitespaceAndComments()
	if quantifier, ok := p.readStepQuantifier(); ok {
		step.quantifier = quantifier
		p.skipWhitespaceAndComments()
	}
	for p.pos < len(p.input) && p.input[p.pos] == '@' {
		capName, err := p.readCapture()
		if err != nil {
			return nil, err
		}
		p.addCaptureToStep(&step, p.ensureCapture(capName))
		p.skipWhitespaceAndComments()
	}

	return &Pattern{steps: []QueryStep{step}}, nil
}

func (p *queryParser) parseFieldShorthandPattern(depth int) (*Pattern, error) {
	fieldName, err := p.readIdentifier()
	if err != nil {
		return nil, err
	}
	p.skipWhitespaceAndComments()
	if p.pos >= len(p.input) || p.input[p.pos] != ':' {
		return nil, fmt.Errorf("query: unexpected identifier %q at position %d", fieldName, p.pos)
	}
	p.pos++ // consume ':'
	p.skipWhitespaceAndComments()

	fieldID, err := p.resolveField(fieldName, 0, 0)
	if err != nil {
		return nil, err
	}

	childPat, err := p.parsePatternElement(depth+1, 0)
	if err != nil {
		return nil, err
	}
	if len(childPat.steps) > 0 {
		childPat.steps[0].field = fieldID
	}

	// Use a wildcard root so field constraints can still be represented in the
	// existing matcher shape.
	root := QueryStep{
		symbol:    0,
		isNamed:   false,
		captureID: -1,
		depth:     depth,
	}
	pat := &Pattern{steps: []QueryStep{root}}
	pat.steps = append(pat.steps, childPat.steps...)
	pat.predicates = append(pat.predicates, childPat.predicates...)
	return pat, nil
}

func (p *queryParser) parsePredicate() (QueryPredicate, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '(' {
		return QueryPredicate{}, fmt.Errorf("query: expected '(' at position %d", p.pos)
	}
	p.pos++ // consume '('
	p.skipWhitespaceAndComments()

	name, err := p.readPredicateName()
	if err != nil {
		return QueryPredicate{}, err
	}

	switch name {
	case "#eq?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		pred := QueryPredicate{
			kind:        predicateEq,
			leftCapture: left,
		}
		if rightIsCapture {
			pred.rightCapture = right
		} else {
			pred.literal = right
		}
		return pred, nil

	case "#not-eq?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		pred := QueryPredicate{
			kind:        predicateNotEq,
			leftCapture: left,
		}
		if rightIsCapture {
			pred.rightCapture = right
		} else {
			pred.literal = right
		}
		return pred, nil

	case "#match?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		if rightIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #match? second argument must be a string literal")
		}
		rx, err := regexp.Compile(right)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid regex in #match?: %w", err)
		}
		return QueryPredicate{
			kind:        predicateMatch,
			leftCapture: left,
			literal:     right,
			regex:       rx,
		}, nil

	case "#not-match?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		if rightIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #not-match? second argument must be a string literal")
		}
		rx, err := regexp.Compile(right)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid regex in #not-match?: %w", err)
		}
		return QueryPredicate{
			kind:        predicateNotMatch,
			leftCapture: left,
			literal:     right,
			regex:       rx,
		}, nil

	case "#any-eq?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		pred := QueryPredicate{
			kind:        predicateAnyEq,
			leftCapture: left,
		}
		if rightIsCapture {
			pred.rightCapture = right
		} else {
			pred.literal = right
		}
		return pred, nil

	case "#any-not-eq?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		pred := QueryPredicate{
			kind:        predicateAnyNotEq,
			leftCapture: left,
		}
		if rightIsCapture {
			pred.rightCapture = right
		} else {
			pred.literal = right
		}
		return pred, nil

	case "#any-match?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		if rightIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #any-match? second argument must be a string literal")
		}
		rx, err := regexp.Compile(right)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid regex in #any-match?: %w", err)
		}
		return QueryPredicate{
			kind:        predicateAnyMatch,
			leftCapture: left,
			literal:     right,
			regex:       rx,
		}, nil

	case "#any-not-match?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		if rightIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #any-not-match? second argument must be a string literal")
		}
		rx, err := regexp.Compile(right)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid regex in #any-not-match?: %w", err)
		}
		return QueryPredicate{
			kind:        predicateAnyNotMatch,
			leftCapture: left,
			literal:     right,
			regex:       rx,
		}, nil

	case "#lua-match?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		p.skipWhitespaceAndComments()
		right, rightIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		if rightIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #lua-match? second argument must be a string literal")
		}
		rx, err := compileLuaPattern(right)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid lua pattern in #lua-match?: %w", err)
		}
		return QueryPredicate{
			kind:        predicateLuaMatch,
			leftCapture: left,
			literal:     right,
			regex:       rx,
		}, nil

	case "#any-of?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		var values []string
		for {
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) {
				return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
			}
			if p.input[p.pos] == ')' {
				p.pos++ // consume ')'
				break
			}
			v, kind, err := p.readPredicateToken()
			if err != nil {
				return QueryPredicate{}, err
			}
			if kind == predicateArgCapture {
				return QueryPredicate{}, fmt.Errorf("query: #any-of? arguments after first must be non-capture literals")
			}
			values = append(values, v)
		}
		if len(values) == 0 {
			return QueryPredicate{}, fmt.Errorf("query: #any-of? requires at least one string literal")
		}
		return QueryPredicate{
			kind:        predicateAnyOf,
			leftCapture: left,
			values:      values,
		}, nil

	case "#not-any-of?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		var values []string
		for {
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) {
				return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
			}
			if p.input[p.pos] == ')' {
				p.pos++ // consume ')'
				break
			}
			v, kind, err := p.readPredicateToken()
			if err != nil {
				return QueryPredicate{}, err
			}
			if kind == predicateArgCapture {
				return QueryPredicate{}, fmt.Errorf("query: #not-any-of? arguments after first must be non-capture literals")
			}
			values = append(values, v)
		}
		if len(values) == 0 {
			return QueryPredicate{}, fmt.Errorf("query: #not-any-of? requires at least one literal")
		}
		return QueryPredicate{
			kind:        predicateNotAnyOf,
			leftCapture: left,
			values:      values,
		}, nil

	case "#has-ancestor?", "#not-has-ancestor?", "#not-has-parent?":
		p.skipWhitespaceAndComments()
		left, leftIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !leftIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: first predicate argument must be a capture in %s", name)
		}

		var types []string
		for {
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) {
				return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
			}
			if p.input[p.pos] == ')' {
				p.pos++ // consume ')'
				break
			}
			tok, kind, err := p.readPredicateToken()
			if err != nil {
				return QueryPredicate{}, err
			}
			if kind == predicateArgCapture {
				return QueryPredicate{}, fmt.Errorf("query: %s node type arguments must be non-capture identifiers", name)
			}
			types = append(types, tok)
		}
		if len(types) == 0 {
			return QueryPredicate{}, fmt.Errorf("query: %s requires at least one node type name", name)
		}
		kind := predicateHasAncestor
		if name == "#not-has-ancestor?" {
			kind = predicateNotHasAncestor
		}
		if name == "#not-has-parent?" {
			kind = predicateNotHasParent
		}
		return QueryPredicate{
			kind:        kind,
			leftCapture: left,
			values:      types,
		}, nil

	case "#is?", "#is-not?":
		var args []struct {
			value string
			kind  predicateArgKind
		}
		for {
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) {
				return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
			}
			if p.input[p.pos] == ')' {
				p.pos++ // consume ')'
				break
			}
			tok, kind, err := p.readPredicateToken()
			if err != nil {
				return QueryPredicate{}, err
			}
			args = append(args, struct {
				value string
				kind  predicateArgKind
			}{value: tok, kind: kind})
		}
		if len(args) == 0 {
			return QueryPredicate{}, fmt.Errorf("query: %s requires arguments", name)
		}

		pred := QueryPredicate{}
		if name == "#is?" {
			pred.kind = predicateIs
		} else {
			pred.kind = predicateIsNot
		}

		if args[0].kind == predicateArgCapture {
			pred.leftCapture = args[0].value
			if len(args) < 2 {
				return QueryPredicate{}, fmt.Errorf("query: %s capture form requires a property argument", name)
			}
			if args[1].kind == predicateArgCapture {
				return QueryPredicate{}, fmt.Errorf("query: %s property argument cannot be a capture", name)
			}
			pred.property = args[1].value
		} else {
			pred.property = args[0].value
			if len(args) >= 2 {
				if args[1].kind != predicateArgCapture {
					return QueryPredicate{}, fmt.Errorf("query: %s second argument must be a capture when provided", name)
				}
				pred.leftCapture = args[1].value
			}
		}
		return pred, nil

	case "#set!":
		p.skipWhitespaceAndComments()
		key, kind, err := p.readPredicateToken()
		if err != nil {
			return QueryPredicate{}, err
		}
		if kind == predicateArgCapture {
			return QueryPredicate{}, fmt.Errorf("query: #set! key must be a non-capture token")
		}
		var values []string
		for {
			p.skipWhitespaceAndComments()
			if p.pos >= len(p.input) {
				return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
			}
			if p.input[p.pos] == ')' {
				p.pos++
				break
			}
			v, _, err := p.readPredicateToken()
			if err != nil {
				return QueryPredicate{}, err
			}
			values = append(values, v)
		}
		return QueryPredicate{
			kind:    predicateSet,
			literal: key,
			values:  values,
		}, nil

	case "#offset!":
		p.skipWhitespaceAndComments()
		capName, kind, err := p.readPredicateToken()
		if err != nil {
			return QueryPredicate{}, err
		}
		if kind != predicateArgCapture {
			return QueryPredicate{}, fmt.Errorf("query: #offset! first argument must be a capture")
		}
		var nums [4]int
		for i := 0; i < 4; i++ {
			p.skipWhitespaceAndComments()
			tok, tokKind, err := p.readPredicateToken()
			if err != nil {
				return QueryPredicate{}, err
			}
			if tokKind == predicateArgCapture {
				return QueryPredicate{}, fmt.Errorf("query: #offset! numeric arguments must be literals")
			}
			n, convErr := strconv.Atoi(tok)
			if convErr != nil {
				return QueryPredicate{}, fmt.Errorf("query: #offset! invalid integer %q", tok)
			}
			nums[i] = n
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'
		return QueryPredicate{
			kind:        predicateOffset,
			leftCapture: capName,
			offset:      nums,
		}, nil

	case "#select-adjacent!":
		p.skipWhitespaceAndComments()
		items, itemsIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !itemsIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #select-adjacent! first argument must be a capture")
		}

		p.skipWhitespaceAndComments()
		anchor, anchorIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !anchorIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #select-adjacent! second argument must be a capture")
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		return QueryPredicate{
			kind:         predicateSelectAdjacent,
			leftCapture:  items,
			rightCapture: anchor,
		}, nil

	case "#strip!":
		p.skipWhitespaceAndComments()
		capName, capIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if !capIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #strip! first argument must be a capture")
		}

		p.skipWhitespaceAndComments()
		pattern, patternIsCapture, err := p.readPredicateArg()
		if err != nil {
			return QueryPredicate{}, err
		}
		if patternIsCapture {
			return QueryPredicate{}, fmt.Errorf("query: #strip! second argument must be a string literal (regex)")
		}
		p.skipWhitespaceAndComments()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return QueryPredicate{}, fmt.Errorf("query: expected ')' to close predicate at position %d", p.pos)
		}
		p.pos++ // consume ')'

		rx, err := regexp.Compile(pattern)
		if err != nil {
			return QueryPredicate{}, fmt.Errorf("query: invalid regex in #strip!: %w", err)
		}
		return QueryPredicate{
			kind:        predicateStrip,
			leftCapture: capName,
			literal:     pattern,
			regex:       rx,
		}, nil

	default:
		return QueryPredicate{}, fmt.Errorf("query: unsupported predicate %q", name)
	}
}

func compileLuaPattern(pattern string) (*regexp.Regexp, error) {
	var out strings.Builder
	inClass := false

	writeLuaClass := func(ch byte, inClass bool) bool {
		if inClass {
			switch ch {
			case 'a':
				out.WriteString("A-Za-z")
			case 'c':
				out.WriteString("[:cntrl:]")
			case 'd':
				out.WriteString("0-9")
			case 'l':
				out.WriteString("a-z")
			case 'p':
				out.WriteString("[:punct:]")
			case 's':
				out.WriteString("\\s")
			case 'u':
				out.WriteString("A-Z")
			case 'w':
				out.WriteString("A-Za-z0-9")
			case 'x':
				out.WriteString("A-Fa-f0-9")
			case 'z':
				out.WriteString("\\x00")
			default:
				return false
			}
			return true
		}
		switch ch {
		case 'a':
			out.WriteString("[A-Za-z]")
		case 'c':
			out.WriteString("[[:cntrl:]]")
		case 'd':
			out.WriteString("[0-9]")
		case 'l':
			out.WriteString("[a-z]")
		case 'p':
			out.WriteString("[[:punct:]]")
		case 's':
			out.WriteString("\\s")
		case 'u':
			out.WriteString("[A-Z]")
		case 'w':
			out.WriteString("[A-Za-z0-9]")
		case 'x':
			out.WriteString("[A-Fa-f0-9]")
		case 'z':
			out.WriteString("\\x00")
		default:
			return false
		}
		return true
	}

	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '[':
			inClass = true
			out.WriteByte(ch)
		case ']':
			inClass = false
			out.WriteByte(ch)
		case '%':
			if i+1 >= len(pattern) {
				out.WriteString("%")
				continue
			}
			i++
			next := pattern[i]
			if writeLuaClass(next, inClass) {
				continue
			}
			out.WriteString(regexp.QuoteMeta(string(next)))
		default:
			out.WriteByte(ch)
		}
	}

	return regexp.Compile(out.String())
}

func (p *queryParser) validatePatternPredicates(pat *Pattern) error {
	if len(pat.predicates) == 0 {
		return nil
	}
	// Keep validation permissive. Runtime predicate evaluation rejects matches
	// when required captures are missing.
	return nil
}

// readIdentifier reads an identifier (node type name, field name).
// Identifiers can contain letters, digits, underscores, dots, and hyphens.
func (p *queryParser) readPredicateName() (string, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '#' {
		return "", fmt.Errorf("query: expected predicate name at position %d", p.pos)
	}
	start := p.pos
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ')' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			break
		}
		p.pos++
	}
	if p.pos == start {
		return "", fmt.Errorf("query: expected predicate name at position %d", start)
	}
	return p.input[start:p.pos], nil
}

func (p *queryParser) readStepQuantifier() (queryQuantifier, bool) {
	if p.pos >= len(p.input) {
		return queryQuantifierOne, false
	}
	switch p.input[p.pos] {
	case '?':
		p.pos++
		return queryQuantifierZeroOrOne, true
	case '*':
		p.pos++
		return queryQuantifierZeroOrMore, true
	case '+':
		p.pos++
		return queryQuantifierOneOrMore, true
	default:
		return queryQuantifierOne, false
	}
}

type predicateArgKind uint8

const (
	predicateArgCapture predicateArgKind = iota
	predicateArgString
	predicateArgAtom
)

func (p *queryParser) readPredicateToken() (arg string, kind predicateArgKind, err error) {
	if p.pos >= len(p.input) {
		return "", predicateArgAtom, fmt.Errorf("query: expected predicate argument at end of input")
	}

	switch p.input[p.pos] {
	case '@':
		name, err := p.readCapture()
		if err != nil {
			return "", predicateArgAtom, err
		}
		return name, predicateArgCapture, nil
	case '"':
		text, err := p.readString()
		if err != nil {
			return "", predicateArgAtom, err
		}
		return text, predicateArgString, nil
	default:
		start := p.pos
		for p.pos < len(p.input) {
			ch := p.input[p.pos]
			if ch == ')' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				break
			}
			p.pos++
		}
		if p.pos == start {
			return "", predicateArgAtom, fmt.Errorf("query: expected predicate argument at position %d", p.pos)
		}
		return p.input[start:p.pos], predicateArgAtom, nil
	}
}

func (p *queryParser) readPredicateArg() (arg string, isCapture bool, err error) {
	if p.pos >= len(p.input) {
		return "", false, fmt.Errorf("query: expected predicate argument at end of input")
	}

	switch p.input[p.pos] {
	case '@':
		name, err := p.readCapture()
		if err != nil {
			return "", false, err
		}
		return name, true, nil
	case '"':
		text, err := p.readString()
		if err != nil {
			return "", false, err
		}
		return text, false, nil
	default:
		return "", false, fmt.Errorf("query: expected capture or string literal in predicate at position %d", p.pos)
	}
}

func (p *queryParser) readIdentifier() (string, error) {
	start := p.pos
	for p.pos < len(p.input) {
		ch := rune(p.input[p.pos])
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '_' || ch == '.' || ch == '-' || ch == '/' {
			p.pos++
		} else {
			break
		}
	}
	if p.pos == start {
		return "", fmt.Errorf("query: expected identifier at position %d", p.pos)
	}
	return p.input[start:p.pos], nil
}

// readCapture reads a @capture_name token. It consumes the '@' and the name.
func (p *queryParser) readCapture() (string, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '@' {
		return "", fmt.Errorf("query: expected '@' at position %d", p.pos)
	}
	p.pos++ // consume '@'
	name, err := p.readIdentifier()
	if err != nil {
		return "", fmt.Errorf("query: expected capture name after '@': %w", err)
	}
	return name, nil
}

// readString reads a quoted string like "func". Consumes the quotes.
func (p *queryParser) readString() (string, error) {
	if p.pos >= len(p.input) || p.input[p.pos] != '"' {
		return "", fmt.Errorf("query: expected '\"' at position %d", p.pos)
	}
	p.pos++ // consume opening '"'
	var sb strings.Builder
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '\\' && p.pos+1 < len(p.input) {
			p.pos++
			sb.WriteByte(p.input[p.pos])
			p.pos++
			continue
		}
		if ch == '"' {
			p.pos++ // consume closing '"'
			return sb.String(), nil
		}
		sb.WriteByte(ch)
		p.pos++
	}
	return "", fmt.Errorf("query: unterminated string")
}

// skipWhitespaceAndComments skips whitespace and ;-style line comments.
func (p *queryParser) skipWhitespaceAndComments() {
	for p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			p.pos++
			continue
		}
		if ch == ';' {
			// Skip to end of line.
			for p.pos < len(p.input) && p.input[p.pos] != '\n' {
				p.pos++
			}
			continue
		}
		break
	}
}

// resolveSymbol looks up a node type name in the language, returning the
// symbol ID and whether it's a named symbol. Uses Language.SymbolByName
// for O(1) lookup.
func (p *queryParser) resolveSymbol(name string) (Symbol, bool, error) {
	if name == "_" {
		return 0, false, nil
	}
	if name == "ERROR" {
		return errorSymbol, true, nil
	}
	if name == "MISSING" {
		return 0, false, nil
	}

	sym, ok := p.lang.SymbolByName(name)
	if !ok {
		// Some highlight queries use supertype-like names such as
		// "pattern/wildcard". Fall back to the rightmost segment when needed.
		if idx := strings.LastIndex(name, "/"); idx >= 0 && idx+1 < len(name) {
			if fallback, fallbackOK := p.lang.SymbolByName(name[idx+1:]); fallbackOK {
				sym = fallback
				ok = true
			}
		}
	}
	if !ok {
		return 0, false, queryUnknownNodeTypeError{name: name}
	}
	isNamed := false
	if int(sym) < len(p.lang.SymbolMetadata) {
		isNamed = p.lang.SymbolMetadata[sym].Named
	}
	return sym, isNamed, nil
}

// resolveField looks up a field name in the language with compatibility
// fallbacks for grammar/query naming drift.
func (p *queryParser) resolveField(name string, parentSymbol Symbol, parentSymbolHint Symbol) (FieldID, error) {
	fid, ok := p.lang.FieldByName(name)
	if ok {
		return fid, nil
	}

	// Some bundled queries use short field names like "key" while grammars
	// expose prefixed names (for example "option_key"). If parent type is
	// known, try parentName_fieldName first.
	seenSymbols := map[Symbol]struct{}{}
	for _, sym := range []Symbol{parentSymbol, parentSymbolHint} {
		if _, seen := seenSymbols[sym]; seen {
			continue
		}
		seenSymbols[sym] = struct{}{}
		if int(sym) < 0 || int(sym) >= len(p.lang.SymbolNames) {
			continue
		}
		parentName := p.lang.SymbolNames[sym]
		if parentName == "" {
			continue
		}
		candidate := parentName + "_" + name
		if fid, ok := p.lang.FieldByName(candidate); ok {
			return fid, nil
		}
		// Allow nested names like "foo/bar" -> "bar_name".
		if idx := strings.LastIndex(parentName, "/"); idx >= 0 && idx+1 < len(parentName) {
			candidate = parentName[idx+1:] + "_" + name
			if fid, ok := p.lang.FieldByName(candidate); ok {
				return fid, nil
			}
		}
	}

	// As a final fallback, allow a unique *_name suffix match.
	matchID := FieldID(0)
	matchCount := 0
	suffix := "_" + name
	for id, fieldName := range p.lang.FieldNames {
		if fieldName == "" {
			continue
		}
		if strings.HasSuffix(fieldName, suffix) {
			matchID = FieldID(id)
			matchCount++
		}
	}
	if matchCount == 1 {
		return matchID, nil
	}

	return 0, fmt.Errorf("query: unknown field name %q", name)
}

// ensureCapture returns the index for a capture name, adding it if new.
func (p *queryParser) ensureCapture(name string) int {
	for i, cn := range p.q.captures {
		if cn == name {
			return i
		}
	}
	idx := len(p.q.captures)
	p.q.captures = append(p.q.captures, name)
	return idx
}

func (p *queryParser) addCaptureToStep(step *QueryStep, captureID int) {
	if step.captureID < 0 {
		step.captureID = captureID
	}
	step.captureIDs = append(step.captureIDs, captureID)
}

func (p *queryParser) addCaptureToAlternative(alt *alternativeSymbol, captureID int) {
	if alt.captureID < 0 {
		alt.captureID = captureID
	}
	alt.captureIDs = append(alt.captureIDs, captureID)
}

// isIdentStart reports whether a byte can start an identifier.
func isIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}
