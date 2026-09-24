package integration

import "testing"

// Story 52 (RFC-006 §6.2.10, RFC-004 §6.4.1/§6.4.4/§8.1.4): interface
// conversions are checked everywhere "RFC-004 applies without special
// exceptions" — including function bodies. The closure-path builder did
// not share the interfaces/impls tables, so ANUY7006 stayed silent
// inside functions.

// In-function concrete-to-interface conversion without impl — struct.
func TestInFunctionConversionWithoutImplStructReports(t *testing.T) {
	source := "interface Reader {\nRead() int\n}\ntype File struct {\nid int\n}\nfunc (f File) Read() int {\nreturn 1\n}\nfunc run() {\nvar f = File{id: 1}\nvar r Reader = f\nr.Read()\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7006" {
		t.Fatalf("diagnostics = %#v, want one ANUY7006", result.Diagnostics)
	}
}

// In-function concrete-to-interface conversion without impl — enum
// (§6.2.10: methods alone do not create conformance).
func TestInFunctionConversionWithoutImplEnumReports(t *testing.T) {
	source := "type Color enum {\nRed\n}\nfunc (c Color) String() string {\nreturn \"red\"\n}\ninterface Stringer {\nString() string\n}\nfunc run() {\nvar c Color = Color.Red\nvar s Stringer = c\ns.String()\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7006" {
		t.Fatalf("diagnostics = %#v, want one ANUY7006", result.Diagnostics)
	}
}

// Top-level guard: the story 40 surface keeps firing.
func TestTopLevelConversionWithoutImplStillReports(t *testing.T) {
	source := "interface Reader {\nRead() int\n}\ntype File struct {\nid int\n}\nfunc (f File) Read() int {\nreturn 1\n}\nvar f = File{id: 1}\nvar r Reader = f\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7006" {
		t.Fatalf("diagnostics = %#v, want one ANUY7006", result.Diagnostics)
	}
}

// In-function conversion WITH the impl record is valid (§6.2.10).
func TestInFunctionConversionWithImplValid(t *testing.T) {
	source := "type Color enum {\nRed\n}\nfunc (c Color) String() string {\nreturn \"red\"\n}\ninterface Stringer {\nString() string\n}\nimpl Stringer for Color\nfunc use(s Stringer) {\ns.String()\n}\nfunc run() {\nvar c Color = Color.Red\nuse(c)\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// §6.2.9: an enum is an ordinary named type — value and pointer receiver
// methods dispatch through the general RFC-004 rules.
func TestEnumMethodsValueAndPointerReceivers(t *testing.T) {
	source := "type Color enum {\nRed\nGreen\n}\nfunc (c Color) IsWarm() bool {\nreturn false\n}\nfunc (c *Color) Bump() {\n}\nfunc run() {\nvar c Color = Color.Red\nc.IsWarm()\nc.Bump()\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// §6.2.10: an impl over an enum missing the interface method reports
// InterfaceMethodMissing.
func TestEnumImplMissingMethodReports(t *testing.T) {
	source := "type Color enum {\nRed\n}\ninterface Stringer {\nString() string\n}\nimpl Stringer for Color\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7001" {
		t.Fatalf("diagnostics = %#v, want one ANUY7001", result.Diagnostics)
	}
}
