package parser

import "testing"

// Story 53 (RFC-004 §6.3.2): an interface MAY extend other interfaces —
// an identifier-only line inside the interface body is an embedded
// interface.
func TestEmbeddedInterfaceMembersParse(t *testing.T) {
	program, err := Parse("interface Reader {\nRead() int\n}\ninterface ReadWriter {\nReader\nWrite() int\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	var decl *InterfaceDecl
	for i := range program.Statements {
		if program.Statements[i].Interface != nil && program.Statements[i].Interface.Name == "ReadWriter" {
			decl = program.Statements[i].Interface
		}
	}
	if decl == nil {
		t.Fatal("ReadWriter declaration expected")
	}
	if len(decl.Embeds) != 1 || decl.Embeds[0] != "Reader" {
		t.Fatalf("embeds = %v, want [Reader]", decl.Embeds)
	}
	if len(decl.Methods) != 1 || decl.Methods[0].Name != "Write" {
		t.Fatalf("methods = %v, want [Write]", decl.Methods)
	}
}

// Story 53 (§6.3.2): the same interface embedded twice is a parse error.
func TestDuplicateEmbedRejects(t *testing.T) {
	if _, err := Parse("interface Reader {\nRead() int\n}\ninterface RW {\nReader\nReader\n}\n"); err == nil {
		t.Fatal("duplicate embedded interface must be rejected")
	}
}

// Story 53 (§6.3.2): an embedded name colliding with a direct method
// name is a parse error — the semantic conflict check covers signatures.
func TestEmbedCollidingWithMethodNameRejects(t *testing.T) {
	if _, err := Parse("interface Reader {\nRead() int\n}\ninterface RW {\nReader\nRead() string\n}\n"); err == nil {
		t.Fatal("embed colliding with a direct method name must be rejected")
	}
}
