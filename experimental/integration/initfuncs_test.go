package integration

import (
	"strings"
	"testing"
)

func hasOnlyCode(result Result, code string) bool {
	if len(result.Diagnostics) != 1 {
		return false
	}
	return string(result.Diagnostics[0].Code) == code
}

func joinedCodes(result Result) string {
	var out []string
	for _, diag := range result.Diagnostics {
		out = append(out, string(diag.Code))
	}
	return strings.Join(out, ",")
}

// RFC-024 §6.9/§8.5: the init function carries no parameters.
func TestInitFuncSignatureParamsReject(t *testing.T) {
	result, err := AnalyzeSource("func init(x int) {\n}")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY3007") {
		t.Fatalf("diagnostics = %s, want one ANUY3007", joinedCodes(result))
	}
}

// §6.9/§8.5: no results either - the `error?` spelling included.
func TestInitFuncSignatureResultReject(t *testing.T) {
	result, err := AnalyzeSource("func init() error? {\nreturn nil\n}")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY3007") {
		t.Fatalf("diagnostics = %s, want one ANUY3007", joinedCodes(result))
	}
}

// §6.9/§8.6: the special init creates no package binding - an explicit
// call rejects (§8.6) instead of resolving to an ordinary function.
func TestInitFuncNotCallable(t *testing.T) {
	result, err := AnalyzeSource("func init() {\n}\ninit()")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY3008") {
		t.Fatalf("diagnostics = %s, want one ANUY3008", joinedCodes(result))
	}
}

// §8.6: a function-value reference rejects the same way.
func TestInitFuncNotAValue(t *testing.T) {
	result, err := AnalyzeSource("func init() {\n}\nvar f = init\nf")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY3008") {
		t.Fatalf("diagnostics = %s, want one ANUY3008", joinedCodes(result))
	}
}

// §8.6: a bare read reports the dedicated code instead of the ordinary
// undefined-name verdict (ANUY2001).
func TestInitFuncNotARead(t *testing.T) {
	result, err := AnalyzeSource("func init() {\n}\ninit")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY3008") {
		t.Fatalf("diagnostics = %s, want one ANUY3008 (no ANUY2001)", joinedCodes(result))
	}
}

// §6.9: a bare reference in a package-variable initializer rejects too.
func TestInitFuncReferenceInPackageVarInitializer(t *testing.T) {
	result, err := AnalyzeSource("func init() {\n}\nvar x = init\n")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY3008") {
		t.Fatalf("diagnostics = %s, want one ANUY3008", joinedCodes(result))
	}
}

// §6.10/§6.22 v2: the init body runs after the variable phase - package
// variables are read and written as ordinary initialized state.
func TestInitBodySeesPackageVars(t *testing.T) {
	result, err := AnalyzeSource("var Greeting = \"unset\"\nfunc init() {\nGreeting = \"from init\"\n}\nGreeting")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// §6.22 v2: declaration order is not observable - a body declared before
// the variable still reads it initialized.
func TestInitBodySeesPackageVarsBeforeDeclaration(t *testing.T) {
	result, err := AnalyzeSource("func init() {\nGreeting\n}\nvar Greeting = \"hello\"")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// §6.9 v3: a local binding named init is ordinary - it shadows the
// special role and the call resolves to the local.
func TestLocalInitShadows(t *testing.T) {
	src := "func init() {\n}\nfunc main_work() {\nvar init = func() {\n}\ninit()\n}\nmain_work()"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none (local shadowing)", joinedCodes(result))
	}
}

// §6.9 v3: a method named init is an ordinary method.
func TestMethodInitOrdinary(t *testing.T) {
	src := "type T struct {\n}\nfunc (t T) init() {\n}\nvar t = T{}\nt.init()"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// §6.9 v3: with no init function declared, an unresolved `init` follows
// the ordinary undefined-name policy of its position - a call statement
// reports the undefined-name code (ANUY2001), value idents keep the
// F-64-1 tolerance.
func TestInitReferenceWithoutDeclarationOrdinary(t *testing.T) {
	result, err := AnalyzeSource("init()")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2001") {
		t.Fatalf("diagnostics = %s, want one ANUY2001 (ordinary undefined name)", joinedCodes(result))
	}
	result, err = AnalyzeSource("var x = init\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none (value-ident tolerance)", joinedCodes(result))
	}
}

// §6.22 v2: the special role is package-wide - a reference in one file
// rejects when the init function is declared in another.
func TestInitDeclaredAcrossFiles(t *testing.T) {
	files := []SourceFile{
		{Path: "a.anuy", Source: "init()"},
		{Path: "b.anuy", Source: "func init() {\n}"},
	}
	result, err := AnalyzeFiles(files)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3008") {
		t.Fatalf("diagnostics = %s, want ANUY3008", joinedCodes(result))
	}
}
