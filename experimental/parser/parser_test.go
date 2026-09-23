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
	// Story 45 (ADR-0011, RFC-004 §6.1.2): the Go receiver form with a
	// receiver binding.
	program, err := Parse("func (u User) age() int {\n}\n")
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
	if s.Method != "User" || s.MethodPointer {
		t.Fatalf("Method/MethodPointer = %q/%v, want User/false", s.Method, s.MethodPointer)
	}
	if s.ReceiverName != "u" {
		t.Fatalf("ReceiverName = %q, want %q", s.ReceiverName, "u")
	}
	if s.ReceiverType == nil || s.ReceiverType.Kind != NamedType || s.ReceiverType.Name != "User" {
		t.Fatalf("ReceiverType = %#v, want the named type User", s.ReceiverType)
	}
	if !s.HasResult || s.ResultNullable {
		t.Fatalf("HasResult/ResultNullable = %v/%v, want true/false", s.HasResult, s.ResultNullable)
	}
}

func TestParseMethodDeclarationNullableResult(t *testing.T) {
	program, err := Parse("func (u User) manager() User? {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if !s.HasResult || !s.ResultNullable {
		t.Fatalf("HasResult/ResultNullable = %v/%v, want true/true", s.HasResult, s.ResultNullable)
	}
}

func TestParseMethodWithArguments(t *testing.T) {
	program, err := Parse("func (u User) deposit(x int) {\n}\n")
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

func TestParsePointerReceiverMethod(t *testing.T) {
	// Story 45 (RFC-004 §6.1.2/§6.2.1): `func (f *File) m()` - the
	// pointer-receiver spelling feeds the method set; the `?` suffix
	// carries the nullable pointer.
	program, err := Parse("func (f *File) Read(d Data) Data {\nreturn d\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Method != "File" || !statement.MethodPointer || statement.Names[0] != "Read" {
		t.Fatalf("method = %+v, want pointer receiver File.Read", statement)
	}
	if statement.ReceiverName != "f" {
		t.Fatalf("ReceiverName = %q, want %q", statement.ReceiverName, "f")
	}
	if statement.ReceiverType == nil || statement.ReceiverType.Kind != PointerType {
		t.Fatalf("ReceiverType = %#v, want the pointer type", statement.ReceiverType)
	}
	value, err := Parse("func (f File) Read(d Data) Data {\nreturn d\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if value.Statements[0].MethodPointer {
		t.Fatal("value receiver parsed as pointer")
	}
	nullable, err := Parse("func (f *File?) Touch() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	rt := nullable.Statements[0].ReceiverType
	if rt == nil || rt.Kind != PointerType || !rt.Nullable {
		t.Fatalf("ReceiverType = %#v, want the nullable pointer", rt)
	}
}

func TestParseUnnamedAndBlankReceiver(t *testing.T) {
	// Story 45 (RFC-004 §6.1.7): unnamed and blank receivers are valid Go
	// forms - they create no binding in the body scope.
	for _, source := range []string{
		"func (*File) M() {\n}\n",
		"func (_ *File) M() {\n}\n",
		"func (File) M() {\n}\n",
	} {
		program, err := Parse(source)
		if err != nil {
			t.Fatalf("%q: %v", source, err)
		}
		if program.Statements[0].ReceiverName != "" {
			t.Fatalf("%q: ReceiverName = %q, want empty", source, program.Statements[0].ReceiverName)
		}
		if program.Statements[0].Method != "File" {
			t.Fatalf("%q: Method = %q, want File", source, program.Statements[0].Method)
		}
	}
}

func TestParseDottedMethodFormRejected(t *testing.T) {
	// Story 45 (ADR-0011): the transitional `func [*]T.m` form is gone -
	// the rejection names the receiver form.
	for _, source := range []string{"func File.Read() {\n}\n", "func *File.Read() {\n}\n"} {
		_, err := Parse(source)
		if err == nil {
			t.Fatalf("%q accepted, want a rejection", source)
		}
		if !strings.Contains(err.Error(), "receiver") {
			t.Fatalf("%q: error = %v, want a receiver-form hint", source, err)
		}
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
		{"func (u User) find() int {\n}\n", "int"},
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

func TestParseEmbedDeclaration(t *testing.T) {
	// Story 29 (RFC-014 6.9): `embed` is a contextual keyword of the
	// struct body; the derived field name is the type name, `*` stripped.
	program, err := Parse("type Server struct {\nembed Logger\naddress string\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fields := program.Statements[0].Struct.Fields
	if len(fields) != 2 || fields[0].Name != "Logger" || !fields[0].Embedded {
		t.Fatalf("fields = %+v, want embedded Logger + address", fields)
	}
	if fields[0].TypeExpr.Canonical() != "Logger" {
		t.Fatalf("type = %q, want Logger", fields[0].TypeExpr.Canonical())
	}
	if fields[1].Embedded {
		t.Fatalf("field 1 = %+v, want an ordinary field", fields[1])
	}
	program, err = Parse("type Server struct {\nembed *Logger\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fields = program.Statements[0].Struct.Fields
	if fields[0].Name != "Logger" || !fields[0].Embedded || fields[0].TypeExpr.Canonical() != "*Logger" {
		t.Fatalf("fields = %+v, want embedded *Logger as Logger", fields)
	}
}

func TestParseEmbedContextualOutsideStruct(t *testing.T) {
	// Outside struct bodies `embed` stays an ordinary identifier.
	program, err := Parse("var embed = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if program.Statements[0].Names[0] != "embed" {
		t.Fatalf("names = %v, want embed", program.Statements[0].Names)
	}
}

func TestParseEmbedRejects(t *testing.T) {
	// RFC-014 6.10: nullable embedding MUST NOT; 6.9: derived names must
	// not collide; the bare type form stays rejected.
	if _, err := Parse("type Server struct {\nembed Logger?\n}\n"); err == nil {
		t.Fatal("nullable embedding accepted")
	}
	if _, err := Parse("type Server struct {\nembed Logger\nLogger string\n}\n"); err == nil {
		t.Fatal("derived name collision accepted")
	}
	if _, err := Parse("type Server struct {\nLogger\n}\n"); err == nil {
		t.Fatal("bare type embedding accepted")
	}
}

func TestParseEnumDeclaration(t *testing.T) {
	// Story 30 (RFC-006 6.1, ADR syntax `type N enum`): a line-based
	// closed set of bare variant names.
	program, err := Parse("type Color enum {\nRed\nGreen\nBlue\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[0]
	if s.Kind != TypeDecl || s.Enum == nil {
		t.Fatalf("kind = %v, want TypeDecl with Enum", s.Kind)
	}
	if s.Enum.Name != "Color" || len(s.Enum.Variants) != 3 {
		t.Fatalf("enum = %+v, want Color with 3 variants", s.Enum)
	}
	if s.Enum.Variants[0].Name != "Red" || s.Enum.Variants[2].Name != "Blue" {
		t.Fatalf("variants = %+v, want [Red Green Blue]", s.Enum.Variants)
	}
	if s.Enum.Variants[0].Span.Start > s.Enum.Variants[2].Span.Start {
		t.Fatalf("variant order not preserved: %+v", s.Enum.Variants)
	}
}

func TestParseEnumSkipsFillerLines(t *testing.T) {
	// Blank lines and `//` comments between variants are skipped.
	program, err := Parse("type Color enum {\n\n// the first\nRed\n\nGreen\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if variants := program.Statements[0].Enum.Variants; len(variants) != 2 {
		t.Fatalf("variants = %+v, want [Red Green]", variants)
	}
}

func TestParseEnumRejects(t *testing.T) {
	// RFC-006 6.1.4/6.1.5/6.1.6: unique bare variants, at least one.
	if _, err := Parse("type Color enum {\nRed\nRed\n}\n"); err == nil {
		t.Fatal("duplicate variant accepted")
	}
	if _, err := Parse("type Color enum {\n}\n"); err == nil {
		t.Fatal("empty enum accepted")
	}
	if _, err := Parse("type Token enum {\nIdentifier(string)\n}\n"); err == nil {
		t.Fatal("payload variant accepted")
	}
}

func TestParseSwitchDeclaration(t *testing.T) {
	// Story 31 (RFC-006 6.3): `switch <binding> { case Enum.Variant: … }`
	// - exhaustive arms over an enum binding.
	program, err := Parse("var c = Color.Red\nswitch c {\ncase Color.Red:\nvar x = 1\ncase Color.Green:\nvar x = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	s := program.Statements[1]
	if s.Kind != Switch || s.Switch == nil {
		t.Fatalf("kind = %v, want Switch", s.Kind)
	}
	if s.Switch.Scrutinee != "c" || len(s.Switch.Arms) != 2 {
		t.Fatalf("switch = %+v, want scrutinee c with 2 arms", s.Switch)
	}
	if s.Switch.Arms[0].Receiver != "Color" || s.Switch.Arms[0].Variant != "Red" || len(s.Switch.Arms[0].Body) != 1 {
		t.Fatalf("arm 0 = %+v", s.Switch.Arms[0])
	}
}

func TestParseSwitchRejects(t *testing.T) {
	// RFC-006 6.3.6-6.3.7: no wildcard/default - the API evolution
	// guarantee; 6.4.7: no guards. A case line without `:` rejects.
	if _, err := Parse("switch c {\ncase Color.Red:\nvar x = 1\ndefault:\nvar x = 2\n}\n"); err == nil {
		t.Fatal("default arm accepted")
	}
	if _, err := Parse("switch c {\ncase Color.Red, Color.Green:\nvar x = 1\n}\n"); err == nil {
		t.Fatal("multi-pattern arm accepted")
	}
	if _, err := Parse("switch c {\ncase Color.Red if x:\nvar x = 1\n}\n"); err == nil {
		t.Fatal("guard accepted")
	}
	if _, err := Parse("switch c {\ncase Color.Red\nvar x = 1\n}\n"); err == nil {
		t.Fatal("case without a colon accepted")
	}
}

func TestParseValueSwitch(t *testing.T) {
	// Story 32 (RFC-006 6.4.2-6.4.4): a value-producing switch RHS with
	// one value line per arm; idents carry the scrutinee and arm reads.
	program, err := Parse("var text string = switch c {\ncase Color.Red:\nx\ncase Color.Green:\ny\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	v := program.Statements[0].Values[0]
	if v.Switch == nil || !v.Switch.Produces || v.Switch.Scrutinee != "c" || len(v.Switch.Arms) != 2 {
		t.Fatalf("value = %#v, want a producing switch", v)
	}
	if v.Switch.Arms[0].Value == nil || v.Switch.Arms[0].Value.Text != "x" {
		t.Fatalf("arm 0 = %+v", v.Switch.Arms[0])
	}
	if len(v.Idents) != 3 || v.Idents[0] != "c" || v.Idents[1] != "x" || v.Idents[2] != "y" {
		t.Fatalf("idents = %v, want [c x y]", v.Idents)
	}
}

func TestParseValueSwitchNestedRejected(t *testing.T) {
	// An arm value is a single expression line - a nested switch rejects.
	if _, err := Parse("var x = switch c {\ncase Color.Red:\nswitch d {\n}\n}\n"); err == nil {
		t.Fatal("nested switch arm value accepted")
	}
}

func TestParseReturnError(t *testing.T) {
	// Story 34 (RFC-005 §6.4.2): `return error <expr>` - the failure
	// return form; `error` is a contextual keyword in return position.
	program, err := Parse("func Save() error? {\nreturn error nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	body := program.Statements[0].Closure.Body
	if len(body) != 1 || body[0].Kind != Return || !body[0].ErrorReturn {
		t.Fatalf("body = %+v, want a failure return", body)
	}
	if len(body[0].Values) != 1 || body[0].Values[0].Text != "nil" {
		t.Fatalf("values = %#v, want the error expression", body[0].Values)
	}
}

func TestParseTryStatement(t *testing.T) {
	// Story 34 (RFC-005 §6.5.3): error-only `try <call>` - the call plus
	// immediate propagation.
	program, err := Parse("func Save() error? {\ntry Log(\"x\")\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	body := program.Statements[0].Closure.Body
	if len(body) != 2 || body[0].Kind != Try || body[0].Call == nil {
		t.Fatalf("body = %+v, want a Try with a call", body)
	}
	if body[0].Call.Receiver != "Log" {
		t.Fatalf("call = %#v, want Log", body[0].Call)
	}
}

func TestParseSwitchNilArm(t *testing.T) {
	// Story 33 (RFC-006 §6.5): `case nil:` arms the nil case of a
	// nullable-enum switch.
	program, err := Parse("switch c {\ncase nil:\nvar x = 1\ncase Color.Red:\nvar y = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	arms := program.Statements[0].Switch.Arms
	if len(arms) != 2 || !arms[0].NilArm || arms[1].Variant != "Red" {
		t.Fatalf("arms = %+v, want nil arm + Red", arms)
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

func TestParseResultList(t *testing.T) {
	// Story 35 (RFC-005 §6.2.3): the strict fallible signature
	// `(Data, error?)` - the list parses positionally, the trailing
	// element mirrors the single-result fields, and the multi-value
	// success return `return d, nil` fits the declared count (§6.4.1).
	program, err := Parse("func Load() (Data, error?) {\nreturn d, nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	cl := program.Statements[0].Closure
	if len(cl.ResultList) != 2 || cl.ResultList[0].Name != "Data" || cl.ResultList[0].Nullable {
		t.Fatalf("ResultList = %+v, want (Data, error?)", cl.ResultList)
	}
	if cl.ResultList[1].Name != "error" || !cl.ResultList[1].Nullable {
		t.Fatalf("trailing = %+v, want error?", cl.ResultList[1])
	}
	if !cl.HasResult || !cl.ResultNullable {
		t.Fatalf("mirror fields = (%v, %v), want (true, true)", cl.HasResult, cl.ResultNullable)
	}
	body := program.Statements[0].Closure.Body
	if len(body) != 1 || body[0].Kind != Return || len(body[0].Values) != 2 {
		t.Fatalf("body = %+v, want a two-value return", body)
	}
}

func TestParseResultListRequiresTrailingError(t *testing.T) {
	// §6.2.4 slice gate: only the fallible shape parses - non-fallible
	// lists and unusual shapes `(error?, int)` reject at the parse layer.
	if _, err := Parse("func F() (A, B) {\nreturn a, b\n}\n"); err == nil {
		t.Fatal("non-fallible result list accepted")
	}
	if _, err := Parse("func F() (error?, int) {\nreturn e, 1\n}\n"); err == nil {
		t.Fatal("unusual result shape accepted")
	}
	if _, err := Parse("func F() (Data, error?) {\nreturn d\n}\n"); err == nil {
		t.Fatal("return arity mismatch accepted")
	}
}

func TestParseValueTry(t *testing.T) {
	// Story 35 (RFC-005 §6.5.1–6.5.2): the value-try declaration - single
	// and multi-name forms carry the operand call and its arguments.
	program, err := Parse("func F() (Data, error?) {\nvar d = try Load(\"x\")\nvar a, b = try Op()\nreturn d, nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	body := program.Statements[0].Closure.Body
	if len(body) != 3 || body[0].Kind != Var || body[0].TryCall == nil {
		t.Fatalf("body = %+v, want a value-try declaration", body)
	}
	if body[0].TryCall.Receiver != "Load" || len(body[0].Names) != 1 || body[0].Values[0].Text != "\"x\"" {
		t.Fatalf("try = %+v, want Load(\"x\") into one binding", body[0])
	}
	if body[1].TryCall == nil || body[1].TryCall.Receiver != "Op" || len(body[1].Names) != 2 {
		t.Fatalf("try = %+v, want Op() into two bindings", body[1])
	}
}

func TestParseFallibleDestructuring(t *testing.T) {
	// Story 36 (RFC-005 §6.3, §6.9.1): the flat destructuring
	// `var data, err = Load(p)` - two names, one call value; the
	// correlation is a kernel concern, the grammar already accepts it.
	program, err := Parse("func F() (Data, error?) {\nvar data, err = Load(\"x\")\nreturn data, nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	body := program.Statements[0].Closure.Body
	if len(body) != 2 || body[0].Kind != Var || len(body[0].Names) != 2 || body[0].Names[1] != "err" {
		t.Fatalf("body = %+v, want a two-name declaration", body)
	}
	if len(body[0].Values) != 1 || body[0].Values[0].Text != "Load(\"x\")" {
		t.Fatalf("values = %#v, want the single call value", body[0].Values)
	}
}

func TestParseDiscardStatement(t *testing.T) {
	// Story 37 (RFC-005 §6.6.2): the explicit-ignore form - the Discard
	// flag rides on the call statement shape; `discard` without a call
	// rejects, and `discard()` stays a call of a binding named discard.
	program, err := Parse("func F() {\ndiscard Close()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	body := program.Statements[0].Closure.Body
	if len(body) != 1 || body[0].Kind != Call || !body[0].Discard || body[0].Call == nil || body[0].Call.Receiver != "Close" {
		t.Fatalf("body = %+v, want a discarded Close call", body)
	}
	method, err := Parse("func F() {\ndiscard obj.Close(\"x\")\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	mBody := method.Statements[0].Closure.Body
	if len(mBody) != 1 || !mBody[0].Discard || mBody[0].Call == nil || len(mBody[0].Call.Segments) != 1 {
		t.Fatalf("method body = %+v, want a discarded method call", mBody)
	}
	if _, err := Parse("func F() {\ndiscard value\n}\n"); err == nil {
		t.Fatal("discard of a non-call accepted")
	}
	if _, err := Parse("func F() {\ndiscard\n}\n"); err == nil {
		t.Fatal("bare discard accepted")
	}
	call, err := Parse("func F() {\ndiscard()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	cBody := call.Statements[0].Closure.Body
	if len(cBody) != 1 || cBody[0].Kind != Call || cBody[0].Discard || cBody[0].Call == nil || cBody[0].Call.Receiver != "discard" {
		t.Fatalf("cBody = %+v, want a call of the binding named discard", cBody)
	}
}

func TestParseInterfaceDecl(t *testing.T) {
	// Story 39 (RFC-004 §6.1.1): the nominal interface declaration -
	// ordered method signatures without receivers; the optional result
	// mirrors nullability; duplicate method names reject.
	program, err := Parse("interface Reader {\nRead(d Data) Data?\nWrite(b []byte)\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	decl := program.Statements[0].Interface
	if decl == nil || decl.Name != "Reader" || len(decl.Methods) != 2 {
		t.Fatalf("decl = %+v, want Reader with two methods", decl)
	}
	if decl.Methods[0].Name != "Read" || !decl.Methods[0].HasResult || !decl.Methods[0].ResultNullable {
		t.Fatalf("Read = %+v, want nullable Data result", decl.Methods[0])
	}
	if decl.Methods[1].HasResult {
		t.Fatalf("Write = %+v, want no result", decl.Methods[1])
	}
	if _, err := Parse("interface Reader {\nRead(d Data) Data\nRead(x int)\n}\n"); err == nil {
		t.Fatal("duplicate interface method accepted")
	}
	if _, err := Parse("interface Reader {\n}\n"); err != nil {
		t.Fatalf("zero-method interface rejected: %v", err)
	}
}

func TestParseImplStatement(t *testing.T) {
	// Story 39 (RFC-004 §6.1.3): `impl Iface for [*]Type` - no body; both
	// receiver forms parse, malformed forms reject.
	pointer, err := Parse("impl Reader for *File\n")
	if err != nil {
		t.Fatal(err)
	}
	decl := pointer.Statements[0].Impl
	if decl == nil || decl.Interface != "Reader" || decl.Type != "File" || !decl.Pointer {
		t.Fatalf("decl = %+v, want Reader for *File", decl)
	}
	value, err := Parse("impl Reader for File\n")
	if err != nil {
		t.Fatal(err)
	}
	if value.Statements[0].Impl.Pointer {
		t.Fatal("value target parsed as pointer")
	}
	if _, err := Parse("impl Reader\n"); err == nil {
		t.Fatal("impl without target accepted")
	}
	if _, err := Parse("impl Reader File\n"); err == nil {
		t.Fatal("impl without for accepted")
	}
}

func TestParseUnsafeConstructs(t *testing.T) {
	// Story 41 (RFC-007 §6.6.1, §6.7.1, §6.6.13): the unsafe block and
	// the `unsafe func` declaration form; the intrinsic namespace is
	// reserved against shadowing.
	program, err := Parse("unsafe {\nvar x int\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 1 || program.Statements[0].Kind != UnsafeBlock {
		t.Fatalf("program = %#v, want one UnsafeBlock", program.Statements)
	}
	if len(program.Statements[0].Body) != 1 {
		t.Fatalf("unsafe body = %#v, want one statement", program.Statements[0].Body)
	}
	unsafeFunc, err := Parse("unsafe func F(u User) User {\nreturn u\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(unsafeFunc.Statements) != 1 || unsafeFunc.Statements[0].Kind != Function || !unsafeFunc.Statements[0].UnsafeFunc {
		t.Fatalf("unsafeFunc = %#v, want a Function with UnsafeFunc", unsafeFunc.Statements)
	}
	rejects := []string{
		"func assume_non_nil(u User) User {\nreturn u\n}\n",
		"var assume_non_nil = 1\n",
		"var unsafe = 1\n",
		"assume_non_nil()\n",
		"assume_non_nil(a, b)\n",
	}
	for _, source := range rejects {
		if _, err := Parse(source); err == nil {
			t.Fatalf("Parse(%q) accepted, want reject", source)
		}
	}
}
