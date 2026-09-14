package parser

import (
	"errors"
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

func TestParseRejectsBlankIdentifier(t *testing.T) {
	for _, source := range []string{"var v, _ = f()", "_ = 1"} {
		_, err := Parse(source + "\n")
		if err == nil {
			t.Fatalf("Parse(%q) accepted blank identifier", source)
		}
		requireCategory(t, err, UnsupportedSyntax)
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

func TestParseRejectsElseIf(t *testing.T) {
	_, err := Parse("if ready {\nx = 1\n} else if other {\nx = 2\n}\n")
	if err == nil {
		t.Fatal("else if accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
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
