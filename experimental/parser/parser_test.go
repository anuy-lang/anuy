package parser

import (
	"errors"
	"strings"
	"testing"
)

func requireCategory(t *testing.T, err error, want ErrorCategory) {
	t.Helper()
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("err = %v, want parser error", err)
	}
	if perr.Category != want {
		t.Fatalf("category = %q, want %q", perr.Category, want)
	}
}

func requireOffset(t *testing.T, err error, want int) {
	t.Helper()
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("err = %v, want parser error", err)
	}
	if perr.Offset != want {
		t.Fatalf("offset = %d, want %d", perr.Offset, want)
	}
}

func TestParseVarAndAssignmentWithSpans(t *testing.T) {
	program, err := Parse("var x int\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 2 || program.Statements[0].Kind != Var || program.Statements[1].Kind != Assign {
		t.Fatalf("program = %#v", program)
	}
	if program.Statements[1].Span.Start != 10 {
		t.Fatalf("assignment span = %#v", program.Statements[1].Span)
	}
}

func TestParseAcceptsDeclarationForms(t *testing.T) {
	sources := []string{
		"var x int",
		"var x int = 5",
		"var x = 5",
		"var u User?",
		"var x, y = f(), g()",
	}
	for _, source := range sources {
		program, err := Parse(source + "\n")
		if err != nil {
			t.Fatalf("Parse(%q): %v", source, err)
		}
		if len(program.Statements) != 1 || program.Statements[0].Kind != Var {
			t.Fatalf("Parse(%q): program = %#v", source, program)
		}
	}
}

func TestParseRejectsRepeatedNullableSuffix(t *testing.T) {
	cases := []struct {
		source string
		offset int
	}{
		{source: "var value int??\n", offset: 14},
		{source: "var value []User??\n", offset: 17},
		{source: "var value map[string]User??\n", offset: 26},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted repeated nullable suffix", tc.source)
		}
		requireCategory(t, err, UnsupportedSyntax)
		requireOffset(t, err, tc.offset)
	}
}

func TestParseRejectsOrdinarySelectorAfterSafeSelector(t *testing.T) {
	cases := []struct {
		source string
		offset int
	}{
		{source: "var city = user?.address.city\n", offset: 24},
		{source: "var city = user.address?.city.name\n", offset: 29},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted ordinary selector after safe selector", tc.source)
		}
		requireCategory(t, err, UnsupportedSyntax)
		requireOffset(t, err, tc.offset)
	}
}

func TestParseDoesNotLetParenthesesResetSafeTail(t *testing.T) {
	_, err := Parse("var city = (user?.address).city\n")
	if err == nil {
		t.Fatal("parentheses reset the safe-navigation tail")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 26)
}

func TestParseRejectsSafeNavigationAssignmentTargetAtSuffix(t *testing.T) {
	_, err := Parse("user?.name = value\n")
	if err == nil {
		t.Fatal("safe-navigation assignment target accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 4)
}

func TestParseDoesNotClassifySafeNavigationExpressionAsAssignmentTarget(t *testing.T) {
	_, err := Parse("user?.name\n")
	if err == nil {
		t.Fatal("bare safe-navigation expression accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 10)
}

func TestParseBuildsCanonicalNullableTypeExpressions(t *testing.T) {
	cases := []struct {
		source string
		want   string
	}{
		{source: "var value User?\n", want: "User?"},
		{source: "var value []User?\n", want: "[]User?"},
		{source: "var value ([]User)?\n", want: "[]User?"},
		{source: "var value [](User?)\n", want: "[](User?)"},
		{source: "var value map[string]User?\n", want: "(map[string]User)?"},
		{source: "var value (map[string]User)?\n", want: "(map[string]User)?"},
		{source: "var value map[string](User?)\n", want: "map[string](User?)"},
		{source: "var value *User?\n", want: "*User?"},
	}
	for _, tc := range cases {
		program, err := Parse(tc.source)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.source, err)
		}
		typeExpr := program.Statements[0].TypeExpr
		if typeExpr == nil {
			t.Fatalf("Parse(%q) did not build a type expression", tc.source)
		}
		if got := typeExpr.Canonical(); got != tc.want {
			t.Fatalf("Parse(%q).TypeExpr.Canonical() = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestParseBuildsNullableTypeExpressionForClosureParameter(t *testing.T) {
	program, err := Parse("var handler = func(value map[string](User?)) {\nvalue\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	param := program.Statements[0].Values[0].Closure.Params[0]
	if param.TypeExpr == nil {
		t.Fatal("closure parameter did not build a type expression")
	}
	if got := param.TypeExpr.Canonical(); got != "map[string](User?)" {
		t.Fatalf("parameter canonical type = %q, want %q", got, "map[string](User?)")
	}
}

func TestParseBuildsNavigationSegmentsAndReceiverIdents(t *testing.T) {
	program, err := Parse("var city = user.address?.city\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if len(value.Idents) != 1 || value.Idents[0] != "user" {
		t.Fatalf("idents = %#v, want only receiver user", value.Idents)
	}
	if value.Navigation == nil {
		t.Fatal("value did not build a navigation expression")
	}
	segments := value.Navigation.Segments
	if len(segments) != 2 {
		t.Fatalf("segments = %#v, want two segments", segments)
	}
	if segments[0].Name != "address" || segments[0].Safe {
		t.Fatalf("first segment = %#v, want ordinary address", segments[0])
	}
	if segments[1].Name != "city" || !segments[1].Safe {
		t.Fatalf("second segment = %#v, want safe city", segments[1])
	}
}

func TestParseRejectsCommaInParenthesizedExpression(t *testing.T) {
	_, err := Parse("var value = (left, right)\n")
	if err == nil {
		t.Fatal("parenthesized comma expression accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 17)
}

func TestParseNavigationCallArgumentsExcludeMemberNames(t *testing.T) {
	program, err := Parse("var result = user?.method(address.city, postcode)\n")
	if err != nil {
		t.Fatal(err)
	}
	idents := program.Statements[0].Values[0].Idents
	want := []string{"user", "address", "postcode"}
	if len(idents) != len(want) {
		t.Fatalf("idents = %#v, want %#v", idents, want)
	}
	for i := range want {
		if idents[i] != want[i] {
			t.Fatalf("idents = %#v, want %#v", idents, want)
		}
	}
}

func TestParseRejectsUnsafeNavigationTailInsideAdditiveExpression(t *testing.T) {
	_, err := Parse("var result = prefix + user?.address.city\n")
	if err == nil {
		t.Fatal("unsafe navigation tail inside additive expression accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 35)
}

func TestParseRejectsUnsafeNavigationTailInCallArgument(t *testing.T) {
	_, err := Parse("var result = user?.method(first, other?.address.city)\n")
	if err == nil {
		t.Fatal("unsafe navigation tail in a call argument accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 47)
}

func TestParseBuildsNavigationMetadataThroughParentheses(t *testing.T) {
	program, err := Parse("var city = (user?.address)?.city\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if len(value.Idents) != 1 || value.Idents[0] != "user" {
		t.Fatalf("idents = %#v, want only receiver user", value.Idents)
	}
	if value.Navigation == nil {
		t.Fatal("parenthesized chain did not build a navigation expression")
	}
	if value.Navigation.Receiver != "user" || len(value.Navigation.Segments) != 2 {
		t.Fatalf("navigation = %#v, want receiver user with two segments", value.Navigation)
	}
	if !value.Navigation.Segments[0].Safe || !value.Navigation.Segments[1].Safe {
		t.Fatalf("segments = %#v, want a pure safe tail", value.Navigation.Segments)
	}
}

func TestParseBuildsNavigationMetadataForCondition(t *testing.T) {
	program, err := Parse("if user.address?.city {\nvalue = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if len(statement.CondIdents) != 1 || statement.CondIdents[0] != "user" {
		t.Fatalf("condition idents = %#v, want only receiver user", statement.CondIdents)
	}
	if statement.CondNavigation == nil {
		t.Fatal("condition did not build a navigation expression")
	}
	if statement.CondNavigation.Receiver != "user" || len(statement.CondNavigation.Segments) != 2 {
		t.Fatalf("condition navigation = %#v, want receiver user with two segments", statement.CondNavigation)
	}
}

func TestParseBuildsNavigationMetadataForConditionCallArguments(t *testing.T) {
	program, err := Parse("if user?.method(address.city, postcode) {\nvalue = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	want := []string{"user", "address", "postcode"}
	if len(statement.CondIdents) != len(want) {
		t.Fatalf("condition idents = %#v, want %#v", statement.CondIdents, want)
	}
	for i := range want {
		if statement.CondIdents[i] != want[i] {
			t.Fatalf("condition idents = %#v, want %#v", statement.CondIdents, want)
		}
	}
	if statement.CondNavigation == nil || len(statement.CondNavigation.Segments) != 1 || !statement.CondNavigation.Segments[0].Call {
		t.Fatalf("condition navigation = %#v, want one safe call segment", statement.CondNavigation)
	}
}

func TestParseRejectsUnsafeNavigationTailInCondition(t *testing.T) {
	_, err := Parse("if user?.address.city {\nvalue = 1\n}\n")
	if err == nil {
		t.Fatal("unsafe navigation tail in condition accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 16)
}

func TestParseRecordsNavigationReceiverAndSafeCallSegment(t *testing.T) {
	program, err := Parse("var name = user.address?.city?.name()\n")
	if err != nil {
		t.Fatal(err)
	}
	navigation := program.Statements[0].Values[0].Navigation
	if navigation == nil {
		t.Fatal("value did not build a navigation expression")
	}
	if navigation.Receiver != "user" {
		t.Fatalf("receiver = %q, want %q", navigation.Receiver, "user")
	}
	segments := navigation.Segments
	if len(segments) != 3 {
		t.Fatalf("segments = %#v, want three segments", segments)
	}
	if segments[2].Name != "name" || !segments[2].Safe || !segments[2].Call {
		t.Fatalf("final segment = %#v, want safe name call", segments[2])
	}
}

func TestParseAcceptsMultipleAssignment(t *testing.T) {
	program, err := Parse("x, y = y, x\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Kind != Assign || len(statement.Names) != 2 || len(statement.Values) != 2 {
		t.Fatalf("statement = %#v", statement)
	}
	if statement.Values[0].Text != "y" || statement.Values[1].Text != "x" {
		t.Fatalf("values = %#v", statement.Values)
	}
}

func TestParseCapturesValueSourceText(t *testing.T) {
	program, err := Parse("x = count + 1\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if value.Text != "count + 1" {
		t.Fatalf("value = %#v", value)
	}
	if len(value.Idents) != 1 || value.Idents[0] != "count" {
		t.Fatalf("idents = %#v", value.Idents)
	}
}

func TestParseAcceptsClosureLiteral(t *testing.T) {
	program, err := Parse("var increment = func() {\ncount = count + 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if value.Closure == nil {
		t.Fatalf("value = %#v, want closure", value)
	}
	if len(value.Closure.Params) != 0 || len(value.Closure.Body) != 1 || value.Closure.Body[0].Kind != Assign {
		t.Fatalf("closure = %#v", value.Closure)
	}
}

func TestParseAcceptsClosureWithTypedParams(t *testing.T) {
	program, err := Parse("var f = func(user User, ids []int) {\nuser\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	cl := program.Statements[0].Values[0].Closure
	if cl == nil || len(cl.Params) != 2 {
		t.Fatalf("closure = %#v", cl)
	}
	if cl.Params[0].Name != "user" || cl.Params[0].Type != "User" || cl.Params[1].Type != "[]int" {
		t.Fatalf("params = %#v", cl.Params)
	}
}

func TestParseRejectsClosureWithMultipleBindings(t *testing.T) {
	_, err := Parse("var f, g = func() {\nx = 1\n}\n")
	if err == nil {
		t.Fatal("closure with multiple bindings accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsClosureWithUntypedParam(t *testing.T) {
	_, err := Parse("var f = func(x) {\nx = 1\n}\n")
	if err == nil {
		t.Fatal("untyped closure parameter accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsClosureInCondition(t *testing.T) {
	_, err := Parse("if func() {\nreturn\n} {\nx = 1\n}\n")
	if err == nil {
		t.Fatal("closure in condition accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsBareDeclaration(t *testing.T) {
	for _, source := range []string{"var x", "var x, y"} {
		_, err := Parse(source + "\n")
		if err == nil {
			t.Fatalf("Parse(%q) accepted bare declaration", source)
		}
		requireCategory(t, err, BareDeclaration)
	}
}

func TestParseRejectsTypedMultipleDeclaration(t *testing.T) {
	_, err := Parse("var x, y int = 1, 2\n")
	if err == nil {
		t.Fatal("typed multiple declaration accepted")
	}
	requireCategory(t, err, TypedMultipleDeclaration)
}

func TestParseRejectsShortDeclaration(t *testing.T) {
	_, err := Parse("x := 1\n")
	if err == nil {
		t.Fatal("short declaration accepted")
	}
	requireCategory(t, err, ShortDeclaration)
}

func TestParseRejectsArityMismatch(t *testing.T) {
	_, err := Parse("x, y = 1, 2, 3\n")
	if err == nil {
		t.Fatal("arity mismatch accepted")
	}
	requireCategory(t, err, ArityMismatch)
}

func TestParseAcceptsSingleValueForMultipleTargets(t *testing.T) {
	if _, err := Parse("x, y = f()\n"); err != nil {
		t.Fatalf("multi-value call form rejected: %v", err)
	}
}

func TestParseRejectsDuplicateAssignmentTargets(t *testing.T) {
	_, err := Parse("x, x = 1, 2\n")
	if err == nil {
		t.Fatal("duplicate assignment targets accepted")
	}
	requireCategory(t, err, DuplicateAssignmentTarget)
}

func TestParseAcceptsBlankIdentifierInNameAndTargetLists(t *testing.T) {
	// GB-3 variant A (owner decision 2026-09-15): `_` is accepted as a
	// write-only discard in name and target lists. This supersedes the former
	// blanket reject; read positions remain rejected (see
	// TestParseRejectsBlankIdentifierRead).
	for _, source := range []string{"var v, _ = f()", "_ = 1"} {
		if _, err := Parse(source + "\n"); err != nil {
			t.Fatalf("Parse(%q): %v", source, err)
		}
	}
}

func TestParseRejectsUnsupportedSyntax(t *testing.T) {
	if _, err := Parse("func main() {}\n"); err == nil {
		t.Fatal("unsupported syntax accepted")
	}
}

func TestParseAcceptsIfWithBlock(t *testing.T) {
	program, err := Parse("if ready {\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Kind != If || statement.Cond != "ready" || len(statement.CondIdents) != 1 {
		t.Fatalf("statement = %#v", statement)
	}
	if len(statement.Body) != 1 || statement.Body[0].Kind != Assign || statement.Else != nil {
		t.Fatalf("branches = %#v", statement)
	}
}

func TestParseAcceptsIfElseWithConditions(t *testing.T) {
	program, err := Parse("if x != nil {\nx = 1\n} else {\nx = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Cond != "x != nil" || len(statement.Body) != 1 || len(statement.Else) != 1 {
		t.Fatalf("statement = %#v", statement)
	}
}

func TestParseAcceptsNestedIf(t *testing.T) {
	program, err := Parse("if a {\nif b {\nx = 1\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	outer := program.Statements[0]
	if outer.Kind != If || len(outer.Body) != 1 || outer.Body[0].Kind != If || len(outer.Body[0].Body) != 1 {
		t.Fatalf("statement = %#v", outer)
	}
}

func TestParseRejectsIfWithoutBlock(t *testing.T) {
	_, err := Parse("if ready\n")
	if err == nil {
		t.Fatal("if without block accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseAcceptsElseIf(t *testing.T) {
	// D-03 (owner decision 2026-09-15): `else if` is parse-level sugar for a
	// nested if; this flips the former blanket reject (F-11).
	program, err := Parse("if ready {\nx = 1\n} else if other {\nx = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	outer := program.Statements[0]
	if outer.Kind != If || len(outer.Else) != 1 || outer.Else[0].Kind != If || outer.Else[0].Cond != "other" {
		t.Fatalf("else if chain = %#v", outer)
	}
}

func TestParseRejectsUnbalancedBlock(t *testing.T) {
	if _, err := Parse("if ready {\nx = 1\n"); err == nil {
		t.Fatal("unbalanced block accepted")
	}
}

func TestParseRejectsUnexpectedClosingBrace(t *testing.T) {
	if _, err := Parse("}\n"); err == nil {
		t.Fatal("unexpected closing brace accepted")
	}
}

func TestParseAcceptsLoopForms(t *testing.T) {
	program, err := Parse("for ready {\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	loop := program.Statements[0]
	if len(program.Statements) != 1 || loop.Kind != Loop || loop.Cond != "ready" || len(loop.Body) != 1 {
		t.Fatalf("condition loop = %#v", loop)
	}

	program, err = Parse("for {\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	loop = program.Statements[0]
	if loop.Kind != Loop || loop.Cond != "" || len(loop.Body) != 1 {
		t.Fatalf("infinite loop = %#v", loop)
	}

	program, err = Parse("for user in users {\nx = user\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	loop = program.Statements[0]
	if loop.Kind != Loop || len(loop.Names) != 1 || loop.Names[0] != "user" ||
		len(loop.Values) != 1 || loop.Values[0].Text != "users" ||
		len(loop.Values[0].Idents) != 1 || loop.Values[0].Idents[0] != "users" {
		t.Fatalf("iteration loop = %#v", loop)
	}

	program, err = Parse("for x != nil {\nx = next(x)\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	loop = program.Statements[0]
	if loop.Kind != Loop || loop.Cond != "x != nil" ||
		len(loop.CondIdents) != 1 || loop.CondIdents[0] != "x" {
		t.Fatalf("expression condition loop = %#v", loop)
	}

	// Spec 1-3-1-1 collision resolution: `for <identifier> { ... }` is a
	// condition loop, not an iteration-binding reject.
	program, err = Parse("for user {\nx = user\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	loop = program.Statements[0]
	if loop.Kind != Loop || loop.Cond != "user" {
		t.Fatalf("identifier condition loop = %#v", loop)
	}
}

func TestParseLoopSpanCoversHeaderLine(t *testing.T) {
	program, err := Parse("for ready {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if span := program.Statements[0].Span; span.Start != 0 || span.End != 11 {
		t.Fatalf("loop span = %#v", span)
	}
}

func TestParseAcceptsBreakContinueAndNesting(t *testing.T) {
	program, err := Parse("for {\nx = readValue()\nif valid(x) {\nbreak\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	loop := program.Statements[0]
	if loop.Kind != Loop || len(loop.Body) != 2 || loop.Body[1].Kind != If ||
		len(loop.Body[1].Body) != 1 || loop.Body[1].Body[0].Kind != Break {
		t.Fatalf("break in if in loop = %#v", loop)
	}

	program, err = Parse("for ready {\ncontinue\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if program.Statements[0].Body[0].Kind != Continue {
		t.Fatalf("continue in loop = %#v", program.Statements[0])
	}

	program, err = Parse("for {\nfor i in items {\nif done {\ncontinue\n}\n}\nbreak\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	outer := program.Statements[0]
	if outer.Kind != Loop || len(outer.Body) != 2 || outer.Body[0].Kind != Loop || outer.Body[1].Kind != Break {
		t.Fatalf("nested loops = %#v", outer)
	}
	inner := outer.Body[0]
	if len(inner.Names) != 1 || inner.Names[0] != "i" || inner.Body[0].Kind != If ||
		inner.Body[0].Body[0].Kind != Continue {
		t.Fatalf("inner loop = %#v", inner)
	}
}

func TestParseRejectsMalformedLoopHeaders(t *testing.T) {
	cases := []struct {
		source string
		offset int
	}{
		{source: "for ready\n", offset: 0},
		{source: "for\n", offset: 0},
		{source: "for x = 1 {\n}\n", offset: 6},
		{source: "for a, b {\n}\n", offset: 5},
		{source: "for var x = 1 {\n}\n", offset: 4},
		{source: "for in users {\n}\n", offset: 4},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted malformed loop header", tc.source)
		}
		requireCategory(t, err, UnsupportedSyntax)
		requireOffset(t, err, tc.offset)
	}
}

func TestParseRejectsUnterminatedLoopBlock(t *testing.T) {
	_, err := Parse("for {\nx = 1\n")
	if err == nil {
		t.Fatal("unterminated loop block accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsIterationHeaderErrors(t *testing.T) {
	cases := []struct {
		source string
		offset int
	}{
		{source: "for user in {\n}\n", offset: 12},
		{source: "for user User in users {\n}\n", offset: 14},
		{source: "for i, item in items {\n}\n", offset: 5},
		{source: "for u.name in xs {\n}\n", offset: 11},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted malformed iteration header", tc.source)
		}
		requireCategory(t, err, UnsupportedSyntax)
		requireOffset(t, err, tc.offset)
	}
}

func TestParseRejectsBlankIterationBindingAsRead(t *testing.T) {
	// GB-3 variant A refines the experimental category for `_` in a loop
	// header (spec 1-3-1-1 RM-11): the header reads the blank identifier,
	// which holds no value.
	_, err := Parse("for _ in xs {\n}\n")
	if err == nil {
		t.Fatal("blank iteration binding accepted")
	}
	requireCategory(t, err, BlankIdentifierRead)
	requireOffset(t, err, 4)
}

func TestParseRejectsBreakContinueOutsideLoop(t *testing.T) {
	_, err := Parse("break\n")
	if err == nil {
		t.Fatal("break outside loop accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 0)

	_, err = Parse("continue\n")
	if err == nil {
		t.Fatal("continue outside loop accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)

	_, err = Parse("for {\nvar f = func() {\nbreak\n}\n}\n")
	if err == nil {
		t.Fatal("break inside closure in loop accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 23)
}

func TestParseRejectsLabeledBreak(t *testing.T) {
	_, err := Parse("break outer\n")
	if err == nil {
		t.Fatal("labeled break accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
	requireOffset(t, err, 6)
}

func TestParseAcceptsStandaloneBlock(t *testing.T) {
	program, err := Parse("{\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	blk := program.Statements[0]
	if len(program.Statements) != 1 || blk.Kind != Block || len(blk.Body) != 1 || blk.Body[0].Kind != Assign {
		t.Fatalf("block = %#v", blk)
	}
}

func TestParseAcceptsNestedBlocks(t *testing.T) {
	program, err := Parse("{\nx = 1\n{\ny = 2\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	outer := program.Statements[0]
	if outer.Kind != Block || len(outer.Body) != 2 || outer.Body[0].Kind != Assign || outer.Body[1].Kind != Block {
		t.Fatalf("outer block = %#v", outer)
	}
	if len(outer.Body[1].Body) != 1 || outer.Body[1].Body[0].Kind != Assign {
		t.Fatalf("inner block = %#v", outer.Body[1])
	}
}

func TestParseRejectsUnbalancedStandaloneBlock(t *testing.T) {
	_, err := Parse("{\nx = 1\n")
	if err == nil {
		t.Fatal("unbalanced standalone block accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsTokensAfterBlockOpen(t *testing.T) {
	// The line-oriented block model requires `{` alone on its opening line;
	// single-line `{ x = 1 }` blocks are outside the confirmed grammar.
	_, err := Parse("{ x = 1 }\n")
	if err == nil {
		t.Fatal("single-line block accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseAcceptsBlankDiscardPositions(t *testing.T) {
	sources := []string{
		"var value, _ = f()",
		"var _ = f()",
		"var _ int",
		"_, y = 1, 2",
		"_, _ = f(), g()",
		"_ = 1",
	}
	for _, source := range sources {
		if _, err := Parse(source + "\n"); err != nil {
			t.Fatalf("Parse(%q): %v", source, err)
		}
	}
}

func TestParseRejectsBlankIdentifierRead(t *testing.T) {
	cases := []struct {
		source string
		offset int
	}{
		{source: "x = _\n", offset: 4},
		{source: "_\n", offset: 0},
		{source: "var v = _\n", offset: 8},
		{source: "if _ {\n}\n", offset: 3},
		{source: "for _ {\n}\n", offset: 4},
		{source: "for _ in xs {\n}\n", offset: 4},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted blank identifier read", tc.source)
		}
		requireCategory(t, err, BlankIdentifierRead)
		requireOffset(t, err, tc.offset)
	}
}

func TestParseAcceptsElseIfChainWithFinalElse(t *testing.T) {
	program, err := Parse("if a {\nx = 1\n} else if b {\nx = 2\n} else if c {\nx = 3\n} else {\nx = 4\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	first := program.Statements[0]
	if first.Kind != If || first.Cond != "a" || len(first.Else) != 1 {
		t.Fatalf("first = %#v", first)
	}
	second := first.Else[0]
	if second.Kind != If || second.Cond != "b" || len(second.Else) != 1 {
		t.Fatalf("second = %#v", second)
	}
	third := second.Else[0]
	if third.Kind != If || third.Cond != "c" || len(third.Else) != 1 || third.Else[0].Kind != Assign {
		t.Fatalf("third = %#v", third)
	}
}

func TestParseAcceptsElseIfWithoutFinalElse(t *testing.T) {
	program, err := Parse("if a {\nx = 1\n} else if b {\nx = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	second := program.Statements[0].Else[0]
	if second.Kind != If || second.Cond != "b" || second.Else != nil {
		t.Fatalf("chain without final else = %#v", second)
	}
}

func TestParseRejectsElseIfWithoutBlock(t *testing.T) {
	_, err := Parse("if a {\nx = 1\n} else if b\n")
	if err == nil {
		t.Fatal("else if without block accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsElseIfConditionViolations(t *testing.T) {
	// Existing condition rules apply to the else-if header.
	cases := []struct {
		source   string
		category ErrorCategory
	}{
		{source: "if a {\nx = 1\n} else if _ {\n}\n", category: BlankIdentifierRead},
		{source: "if a {\nx = 1\n} else if x = 1 {\n}\n", category: UnsupportedSyntax},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted invalid else-if condition", tc.source)
		}
		requireCategory(t, err, tc.category)
	}
}

func TestParseAcceptsCallStatements(t *testing.T) {
	// Story 05 variant A (owner decision 2026-09-16): zero-argument call
	// statements — the effectful statement form used by the normative
	// RFC-002 §22/§85 and RFC-003 §85 examples.
	bare, err := Parse("clear()\n")
	if err != nil {
		t.Fatal(err)
	}
	if bare.Statements[0].Kind != Call {
		t.Fatalf("bare call kind = %v", bare.Statements[0].Kind)
	}
	call := bare.Statements[0].Call
	if call == nil || call.Receiver != "clear" || len(call.Segments) != 0 {
		t.Fatalf("bare call = %#v", call)
	}
	navigation, err := Parse("user.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	call = navigation.Statements[0].Call
	if navigation.Statements[0].Kind != Call || call == nil || call.Receiver != "user" ||
		len(call.Segments) != 1 || call.Segments[0].Name != "save" || !call.Segments[0].Call {
		t.Fatalf("navigation call = %#v", call)
	}
	chain, err := Parse("a.b.c()\n")
	if err != nil {
		t.Fatal(err)
	}
	call = chain.Statements[0].Call
	if call == nil || len(call.Segments) != 2 || call.Segments[0].Call || !call.Segments[1].Call {
		t.Fatalf("chain call = %#v", call)
	}
	// Story 07: call statements carry arguments (owner decision 2026-09-17);
	// `clear(x)`/`user.save(x)` flipped from the story 05 rejects.
	withArgs, err := Parse("user.save(x)\n")
	if err != nil {
		t.Fatal(err)
	}
	call = withArgs.Statements[0].Call
	if call == nil || len(call.Segments) != 1 || len(withArgs.Statements[0].Values) != 1 ||
		len(withArgs.Statements[0].Values[0].Idents) != 1 || withArgs.Statements[0].Values[0].Idents[0] != "x" {
		t.Fatalf("call with arguments = %#v / %#v", call, withArgs.Statements[0].Values)
	}
}

func TestParseRejectsCallStatementEdges(t *testing.T) {
	cases := []struct {
		source   string
		category ErrorCategory
	}{
		{source: "_()\n", category: BlankIdentifierRead},
		{source: "clear().field\n", category: UnsupportedSyntax},
		{source: "a.b().c\n", category: UnsupportedSyntax},
		// `user?.save()` flipped to accept in story 08 (Q3-A, RFC-002
		// §33/§42) - pinned by TestParseSafeCallStatement.
		{source: "nil()\n", category: UnsupportedSyntax},
		{source: "func() {\n}\n", category: UnsupportedSyntax},
	}
	for _, tc := range cases {
		_, err := Parse(tc.source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted invalid call statement", tc.source)
		}
		requireCategory(t, err, tc.category)
	}
}

func TestParseMethodDeclaration(t *testing.T) {
	program, err := Parse("func User.age() int {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 1 {
		t.Fatalf("statements = %d, want 1", len(program.Statements))
	}
	s := program.Statements[0]
	if s.Kind != Function || len(s.Names) != 1 || s.Names[0] != "age" {
		t.Fatalf("statement = %#v, want a Function declaration of age", s)
	}
	if s.Method != "User" {
		t.Fatalf("Method = %q, want %q", s.Method, "User")
	}
	if !s.HasResult || s.ResultNullable {
		t.Fatalf("HasResult/ResultNullable = %v/%v, want true/false", s.HasResult, s.ResultNullable)
	}
}

func TestParseMethodDeclarationNullableResult(t *testing.T) {
	program, err := Parse("func User.manager() User? {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if !s.HasResult || !s.ResultNullable {
		t.Fatalf("HasResult/ResultNullable = %v/%v, want true/true", s.HasResult, s.ResultNullable)
	}
}

func TestParseMethodWithArguments(t *testing.T) {
	program, err := Parse("func User.deposit(x int) {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != Function || s.Method != "User" || s.Names[0] != "deposit" {
		t.Fatalf("statement = %#v, want the deposit method on User", s)
	}
	if s.Closure == nil || len(s.Closure.Params) != 1 || s.Closure.Params[0].Name != "x" {
		t.Fatalf("params = %#v, want one parameter x", s.Closure)
	}
}

func TestParseFunctionDeclarationResultType(t *testing.T) {
	program, err := Parse("func find() User? {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != Function || s.Names[0] != "find" || s.Method != "" {
		t.Fatalf("statement = %#v, want the find function", s)
	}
	if !s.HasResult || !s.ResultNullable {
		t.Fatalf("HasResult/ResultNullable = %v/%v, want true/true", s.HasResult, s.ResultNullable)
	}
}

func TestParseRejectsResultTypeOnClosureValue(t *testing.T) {
	// Result types belong to the declaration forms (story 08 Q4-A); a
	// closure literal stays result-less.
	_, err := Parse("var f = func() User {\n}\n")
	if err == nil {
		t.Fatal("closure literal accepted a result type")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseReturnWithValueInResultDeclaration(t *testing.T) {
	program, err := Parse("func find() User {\nreturn u\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != Function || len(s.Closure.Body) != 1 {
		t.Fatalf("statement = %#v, want a function with one body statement", s)
	}
	ret := s.Closure.Body[0]
	if ret.Kind != Return || len(ret.Values) != 1 || ret.Values[0].Text != "u" {
		t.Fatalf("return = %#v, want one value u", ret)
	}
}

func TestParseBareReturnInResultDeclaration(t *testing.T) {
	program, err := Parse("func find() int {\nreturn\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	ret := program.Statements[0].Closure.Body[0]
	if ret.Kind != Return || len(ret.Values) != 0 {
		t.Fatalf("return = %#v, want the bare form", ret)
	}
}

func TestParseRejectsReturnValueWithoutResult(t *testing.T) {
	_, err := Parse("func f() {\nreturn x\n}\n")
	if err == nil {
		t.Fatal("void declaration accepted `return x`")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsReturnValueTopLevel(t *testing.T) {
	_, err := Parse("return x\n")
	if err == nil {
		t.Fatal("top level accepted `return x`")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsReturnValueInClosure(t *testing.T) {
	_, err := Parse("var f = func() {\nreturn x\n}\n")
	if err == nil {
		t.Fatal("closure accepted `return x`")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseSafeCallStatement(t *testing.T) {
	// RFC-002 §33/§42: a safe method call is a valid statement (story 08
	// Q3-A).
	program, err := Parse("u?.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != Call || s.Call == nil || len(s.Call.Segments) != 1 {
		t.Fatalf("statement = %#v, want one safe call segment", s)
	}
	if !s.Call.Segments[0].Safe || !s.Call.Segments[0].Call {
		t.Fatalf("segment = %#v, want a safe call", s.Call.Segments[0])
	}
}

func TestParseFunctionDeclRetainsResultTypeExpr(t *testing.T) {
	// Story 16: the result TypeExpr is retained structurally - lowering
	// cannot spell the Nullable[T] result without it (parseClosure
	// computed and dropped it before).
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"func find() User? {\n}\n", "User?"},
		{"func f() int {\n}\n", "int"},
		{"func User.find() int {\n}\n", "int"},
	} {
		program, err := Parse(tc.source)
		if err != nil {
			t.Fatalf("%q: %v", tc.source, err)
		}
		got := program.Statements[0].Closure.ResultTypeExpr
		if got == nil || got.Canonical() != tc.want {
			t.Fatalf("%q: ResultTypeExpr = %#v, want %q", tc.source, got, tc.want)
		}
	}
	if program, err := Parse("func f() {\n}\n"); err != nil || program.Statements[0].Closure.ResultTypeExpr != nil {
		t.Fatalf("void function: err=%v ResultTypeExpr=%#v, want nil", err, program.Statements[0].Closure.ResultTypeExpr)
	}
}

func TestParseStructDeclaration(t *testing.T) {
	// Story 21 (RFC-014 6.2): Go-syntax struct declarations with ordered
	// typed fields.
	program, err := Parse("type User struct {\nid UserID\nname string\nmanager *User?\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != TypeDecl || s.Struct == nil {
		t.Fatalf("kind = %v, want TypeDecl with Struct", s.Kind)
	}
	if s.Struct.Name != "User" || len(s.Struct.Fields) != 3 {
		t.Fatalf("struct = %+v, want User with 3 fields", s.Struct)
	}
	if s.Struct.Fields[0].Name != "id" || s.Struct.Fields[0].TypeExpr.Canonical() != "UserID" {
		t.Fatalf("field 0 = %+v", s.Struct.Fields[0])
	}
	if s.Struct.Fields[2].Name != "manager" || s.Struct.Fields[2].TypeExpr.Canonical() != "*User?" {
		t.Fatalf("field 2 = %+v", s.Struct.Fields[2])
	}
	if s.Struct.Fields[0].Span.Start > s.Struct.Fields[2].Span.Start {
		t.Fatalf("field order not preserved: %#v", s.Struct.Fields)
	}
}

func TestParseStructZeroField(t *testing.T) {
	// RFC-014 6.2: a zero-field struct is valid.
	program, err := Parse("type Marker struct {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != TypeDecl || s.Struct == nil || s.Struct.Name != "Marker" || len(s.Struct.Fields) != 0 {
		t.Fatalf("struct = %#v, want zero-field Marker", s.Struct)
	}
}

func TestParseStructDuplicateFieldRejected(t *testing.T) {
	// RFC-014 6.2: two direct fields of one struct MUST NOT share a name.
	if _, err := Parse("type User struct {\nid int\nid string\n}\n"); err == nil {
		t.Fatal("duplicate field accepted")
	}
}

func TestParseStructBlankAndReservedFieldNamesRejected(t *testing.T) {
	if _, err := Parse("type User struct {\n_ int\n}\n"); err == nil {
		t.Fatal("blank field name accepted")
	}
	if _, err := Parse("type User struct {\nreturn int\n}\n"); err == nil {
		t.Fatal("reserved field name accepted")
	}
	if _, err := Parse("type User struct {\nname\n}\n"); err == nil {
		t.Fatal("field without a type accepted")
	}
	if _, err := Parse("type User struct {\nname string\n"); err == nil {
		t.Fatal("unclosed struct accepted")
	}
}

func TestParseKeyedLiteralValue(t *testing.T) {
	// RFC-014 6.3-6.4: keyed construction is one value - commas inside the
	// braces do not split it, keys are not binding reads.
	program, err := Parse("var u = User{id: 1, name: \"x\"}\n")
	if err != nil {
		t.Fatal(err)
	}
	v := program.Statements[0].Values[0]
	if v.Text != "User{id: 1, name: \"x\"}" {
		t.Fatalf("text = %q", v.Text)
	}
	if v.Keyed == nil || v.Keyed.Name != "User" {
		t.Fatalf("keyed = %#v, want User", v.Keyed)
	}
	if len(v.Idents) != 0 {
		t.Fatalf("idents = %v, want none (keys and the type name are not reads)", v.Idents)
	}
}

func TestParseKeyedLiteralValueIdents(t *testing.T) {
	// Field values keep their reads, keys do not count; nested literals
	// hide their keys recursively.
	program, err := Parse("var u = User{id: x, name: n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if idents := program.Statements[0].Values[0].Idents; len(idents) != 2 || idents[0] != "x" || idents[1] != "n" {
		t.Fatalf("idents = %v, want [x n]", idents)
	}
	program, err = Parse("var o = Outer{inner: Inner{x: 1}, tag: tag}\n")
	if err != nil {
		t.Fatal(err)
	}
	if idents := program.Statements[0].Values[0].Idents; len(idents) != 1 || idents[0] != "tag" {
		t.Fatalf("idents = %v, want [tag]", idents)
	}
}

func TestParseKeyedLiteralDuplicateKeyRejected(t *testing.T) {
	// RFC-014 6.3: every direct field is initialized exactly once.
	if _, err := Parse("var u = User{id: 1, id: 2}\n"); err == nil {
		t.Fatal("duplicate key accepted")
	}
}

func TestParseKeyedLiteralMultiline(t *testing.T) {
	// Story 24 (RFC-014 6.4): `N{` opens a multiline construction, each
	// entry line ends with a comma, and the bare `}` line closes it; the
	// raw source spans the value text verbatim.
	program, err := Parse("var u = User{\nid: 1,\nname: \"x\",\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	v := program.Statements[0].Values[0]
	if v.Text != "User{\nid: 1,\nname: \"x\",\n}" {
		t.Fatalf("text = %q", v.Text)
	}
	if v.Keyed == nil || v.Keyed.Name != "User" {
		t.Fatalf("keyed = %#v, want User", v.Keyed)
	}
	if fields := v.Keyed.Fields; len(fields) != 2 || fields[0] != "id" || fields[1] != "name" {
		t.Fatalf("fields = %v, want [id name]", fields)
	}
	if len(v.Idents) != 0 {
		t.Fatalf("idents = %v, want none", v.Idents)
	}
}

func TestParseKeyedLiteralMultilineIdentsSkipFiller(t *testing.T) {
	// Blank lines and `//` comment lines inside the literal are skipped
	// but stay in the verbatim text; field values keep their reads in
	// source order.
	source := "var u = User{\n\n// the identifier\nid: x,\n\nname: n,\n}\n"
	program, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	v := program.Statements[0].Values[0]
	if v.Text != "User{\n\n// the identifier\nid: x,\n\nname: n,\n}" {
		t.Fatalf("text = %q", v.Text)
	}
	if idents := v.Idents; len(idents) != 2 || idents[0] != "x" || idents[1] != "n" {
		t.Fatalf("idents = %v, want [x n]", idents)
	}
}

func TestParseKeyedLiteralMultilineNestedSingleLineEntry(t *testing.T) {
	// A single-line nested literal inside an entry keeps its keys hidden.
	program, err := Parse("var o = Outer{\ninner: Inner{x: 1},\ntag: tag,\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if idents := program.Statements[0].Values[0].Idents; len(idents) != 1 || idents[0] != "tag" {
		t.Fatalf("idents = %v, want [tag]", idents)
	}
}

func TestParseKeyedLiteralMultilineAssignAndFieldTargets(t *testing.T) {
	// Assignment and field-mutation RHS open multiline blocks too.
	program, err := Parse("var u User\nu = User{\nid: 1,\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if s := program.Statements[1]; s.Kind != Assign || s.Values[0].Keyed == nil {
		t.Fatalf("assign = %#v, want Assign with a keyed value", s)
	}
	program, err = Parse("var u User\nu.meta = User{\nid: 1,\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[1]
	if s.Kind != Assign || s.Target == nil || s.Values[0].Keyed == nil {
		t.Fatalf("field assign = %#v, want a Target with a keyed value", s)
	}
}

func TestParseKeyedLiteralMultilineRejects(t *testing.T) {
	// RFC-014 6.4: every entry MUST end with a comma; the restricted slice
	// keeps one entry per line, single-line entry values, and a bare `}`
	// closer (nested multiline, two entries, and a suffix closer are
	// explicit follow-ups).
	rejects := []struct{ name, source string }{
		{"missing trailing comma on last entry", "var u = User{\nid: 1,\nname: \"x\"\n}\n"},
		{"missing comma on middle entry", "var u = User{\nid: 1\nname: \"x\",\n}\n"},
		{"two entries on one line", "var u = User{\nid: 1, name: \"x\",\n}\n"},
		{"nested multiline literal", "var o = Outer{\ninner: Inner{\nx: 1,\n},\n}\n"},
		{"closer with suffix", "var u = User{\nid: 1,\n},\n"},
		{"missing closer", "var u = User{\nid: 1,\n"},
	}
	for _, reject := range rejects {
		if _, err := Parse(reject.source); err == nil {
			t.Fatalf("%s: accepted", reject.name)
		}
	}
}

func TestParseFieldMutation(t *testing.T) {
	// Story 22 (RFC-014 6.7): `user.name = "Bob"` — an ordinary field
	// path as the assignment target.
	program, err := Parse("var u User\nu.name = \"Bob\"\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[1]
	if s.Kind != Assign || s.Target == nil {
		t.Fatalf("kind = %v, target = %#v, want Assign with Target", s.Kind, s.Target)
	}
	if s.Target.Receiver != "u" || len(s.Target.Segments) != 1 || s.Target.Segments[0].Name != "name" {
		t.Fatalf("target = %#v, want u.name", s.Target)
	}
	if len(s.Values) != 1 || s.Values[0].Text != "\"Bob\"" {
		t.Fatalf("values = %#v, want the RHS value", s.Values)
	}
}

func TestParseFieldMutationRejects(t *testing.T) {
	// Deeper paths and safe targets stay rejected; the safe-target reject
	// preserves its original form. Story 25 moves the deeper reject to the
	// three-segment path.
	if _, err := Parse("u.f.g.h = 1\n"); err == nil || !strings.Contains(err.Error(), "deeper field path") {
		t.Fatalf("err = %v, want deeper field path reject", err)
	}
	if _, err := Parse("u?.f = 1\n"); err == nil || !strings.Contains(err.Error(), "safe navigation is not an assignment target") {
		t.Fatalf("err = %v, want safe target reject", err)
	}
	if _, err := Parse("u.f\n"); err == nil || !strings.Contains(err.Error(), "call statement") {
		t.Fatalf("err = %v, want the legacy navigation-call reject", err)
	}
}

func TestParseDeepFieldMutation(t *testing.T) {
	// Story 25 (RFC-014 6.7): `u.f.g = expr` — a two-segment ordinary
	// path as the assignment target.
	program, err := Parse("var u User\nu.profile.badge = \"B\"\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[1]
	if s.Kind != Assign || s.Target == nil {
		t.Fatalf("kind = %v, target = %#v, want Assign with Target", s.Kind, s.Target)
	}
	if s.Target.Receiver != "u" || len(s.Target.Segments) != 2 ||
		s.Target.Segments[0].Name != "profile" || s.Target.Segments[1].Name != "badge" {
		t.Fatalf("target = %#v, want u.profile.badge", s.Target)
	}
	if len(s.Values) != 1 || s.Values[0].Text != "\"B\"" {
		t.Fatalf("values = %#v, want the RHS value", s.Values)
	}
}

func TestParseDeepFieldMutationSafeSegmentRejected(t *testing.T) {
	// A safe segment inside the target is not an assignment target.
	if _, err := Parse("u.profile?.badge = 1\n"); err == nil || !strings.Contains(err.Error(), "safe navigation is not an assignment target") {
		t.Fatalf("err = %v, want safe target reject", err)
	}
}
