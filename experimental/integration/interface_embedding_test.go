package integration

import "testing"

// Story 53 (RFC-004 §6.3.2): the effective method set of an embedding
// interface contains the methods of its embedded interfaces — dispatch
// and impl conformance see the union.
func TestEmbeddedInterfaceDispatch(t *testing.T) {
	source := "type File struct {\nid int\n}\nfunc (f File) Read() int {\nreturn f.id\n}\nfunc (f File) Write() int {\nreturn f.id\n}\ninterface Reader {\nRead() int\n}\ninterface Writer {\nWrite() int\n}\ninterface ReadWriter {\nReader\nWriter\n}\nimpl ReadWriter for File\nfunc use(rw ReadWriter) {\nrw.Read()\nrw.Write()\n}\nfunc run() {\nvar f = File{id: 1}\nuse(f)\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// §6.3.3 (the central rule): impls of Reader and Writer do not imply
// ReadWriter — using the value as ReadWriter without the explicit impl
// reports ANUY7006.
func TestEmbeddedExplicitnessRequiresOwnImpl(t *testing.T) {
	source := "type File struct {\nid int\n}\nfunc (f File) Read() int {\nreturn f.id\n}\nfunc (f File) Write() int {\nreturn f.id\n}\ninterface Reader {\nRead() int\n}\ninterface Writer {\nWrite() int\n}\ninterface ReadWriter {\nReader\nWriter\n}\nimpl Reader for File\nimpl Writer for File\nfunc use(rw ReadWriter) {\nrw.Read()\n}\nfunc run() {\nvar f = File{id: 1}\nuse(f)\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7006" {
		t.Fatalf("diagnostics = %#v, want one ANUY7006", result.Diagnostics)
	}
}

// §6.3.2: the same method name with a different signature across
// embedded interfaces is a compile error (ANUY7007).
func TestEmbeddedSignatureConflictReports(t *testing.T) {
	source := "interface Reader {\nRead() int\n}\ninterface Writer {\nRead() string\n}\ninterface ReadWriter {\nReader\nWriter\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7007" {
		t.Fatalf("diagnostics = %#v, want one ANUY7007", result.Diagnostics)
	}
}

// §6.3.2: a direct method identical to an embedded one merges — no
// duplicate, dispatch stays valid.
func TestEmbeddedIdenticalDuplicateMerges(t *testing.T) {
	source := "type File struct {\nid int\n}\nfunc (f File) Read() int {\nreturn f.id\n}\ninterface Reader {\nRead() int\n}\ninterface ReadWriter {\nReader\nRead() int\n}\nimpl ReadWriter for File\nfunc use(rw ReadWriter) {\nrw.Read()\n}\nfunc run() {\nvar f = File{id: 1}\nuse(f)\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// §6.2.2 guard: a pointer-receiver method is callable through an
// addressable value, and the convenience does not change the interface
// method set — the value-target impl reports ANUY7001.
func TestAddressableConvenienceMethodSetGuards(t *testing.T) {
	pos := "type File struct {\nid int\n}\nfunc (f *File) Read() int {\nreturn f.id\n}\nfunc run() {\nvar f = File{id: 1}\nf.Read()\n}\n"
	result, err := AnalyzeSource(pos)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("positive: diagnostics = %#v, want none", result.Diagnostics)
	}
	neg := "type File struct {\nid int\n}\nfunc (f *File) Read() int {\nreturn f.id\n}\ninterface Reader {\nRead() int\n}\nimpl Reader for File\n"
	result, err = AnalyzeSource(neg)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7001" {
		t.Fatalf("negative: diagnostics = %#v, want one ANUY7001", result.Diagnostics)
	}
}

// §6.3.4: a relation between interfaces is created only by embedding —
// `impl Reader for ReadWriter` targets an interface and reports
// InterfaceMethodMissing.
func TestImplOverInterfaceTargetRejects(t *testing.T) {
	source := "interface Reader {\nRead() int\n}\ninterface ReadWriter {\nRead() int\n}\nimpl Reader for ReadWriter\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY7001" {
		t.Fatalf("diagnostics = %#v, want one ANUY7001", result.Diagnostics)
	}
}
