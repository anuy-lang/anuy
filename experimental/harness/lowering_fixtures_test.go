package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anuy-lang/anuy/experimental/lowering"
)

// TestLoweringCoreAcceptFixturesLower runs every lowering-core fixture
// through Lower (story 10, PF-G-06 flip, ADR-0003). Each fixture pins its
// representation marker: the tagged carrier with Some/None conversions for
// value shapes and nullable slices (RFC-002 §6.8.2, §6.8.10–6.8.13), or the
// native Go type without a carrier prelude for native-nil shapes (§6.8.1,
// §6.8.6, §6.9.2). The table length doubles as the fixture counter.
func TestLoweringCoreAcceptFixturesLower(t *testing.T) {
	markers := []struct {
		file      string
		want      string
		noCarrier bool
	}{
		{"nullable-decl.anuy", "var u anuyabi.Nullable[User]", false},
		{"nullable-init-some.anuy", "var x anuyabi.Nullable[int] = anuyabi.Some(42)", false},
		{"nullable-assign-nil.anuy", "u = anuyabi.None[User]()", false},
		{"nullable-copy.anuy", "var b anuyabi.Nullable[int] = a", false},
		{"nullable-closure-param.anuy", "f := func(x anuyabi.Nullable[int])", false},
		{"nullable-slice.anuy", "var s anuyabi.Nullable[[]User]", false},
		{"slice-of-nullable.anuy", "var xs []anuyabi.Nullable[User]", false},
		{"nullable-narrow-call.anuy", "if !u.IsNil() {", false},
		{"safe-call-carrier.anuy", "u.Value.save()", false},
		{"safe-call-native-nil.anuy", "err.Error()", true},
		{"safe-value-carrier.anuy", "c = anuyabi.Some(a.Value.count)", false},
		{"safe-value-native-nil.anuy", "s = anuyabi.Some(err.Error())", false},
		{"nil-condition-carrier.anuy", "if n.IsNil() {", false},
		{"call-arguments.anuy", "u.m(a, b)", true},
		{"safe-call-arguments.anuy", "u.Value.send(payload())", false},
		{"func-decl-carrier.anuy", "func find() anuyabi.Nullable[User]", false},
		{"func-method.anuy", "func (anuyRecv User) find() anuyabi.Nullable[User]", false},
		{"struct-decl.anuy", "type User struct {", true},
		{"struct-construction.anuy", "u := User{id: 1}", true},
		{"struct-construction-multiline.anuy", "u := User{\nid: 1,\nname: \"Ann\",\n}", true},
		{"struct-field-mutation.anuy", "u.name = \"Bob\"", true},
		{"struct-deep-field-mutation.anuy", "u.profile.badge = \"B\"", true},
		{"struct-embed.anuy", "type Server struct {\n\tLogger\n\tport string\n}", true},
		{"enum-declaration.anuy", "ColorRed Color = 1", true},
		{"enum-switch.anuy", "case ColorRed:", true},
		{"enum-switch-value.anuy", "text = \"red\"", true},
		{"enum-switch-nullable.anuy", "case c.IsNil():", false},
		{"error-try.anuy", "if err := Log(\"done\"); err != nil {", true},
		{"error-strict-fallible.anuy", "func Load(path string, cause error) (Data, error) {", true},
		{"error-correlation.anuy", "data, err := LoadData(path)", true},
		{"error-discard.anuy", "Log(\"done\")", true},
		{"interface-reader.anuy", "type Reader interface {", true},
		{"native-nil-pointer.anuy", "var p *User\n\tp = nil", true},
		{"native-nil-map.anuy", "var m map[string]User", true},
		{"native-nil-error.anuy", "var err error", true},
	}
	dir := filepath.Join("..", "fixtures", "lowering-core", "accept")
	files, err := filepath.Glob(filepath.Join(dir, "*.anuy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != len(markers) {
		t.Fatalf("lowering-core accept fixtures = %d, table = %d - update the counter", len(files), len(markers))
	}
	for _, fixture := range markers {
		source, err := os.ReadFile(filepath.Join(dir, fixture.file))
		if err != nil {
			t.Fatal(err)
		}
		got, err := lowering.Lower(string(source))
		if err != nil {
			t.Fatalf("%s: %v", fixture.file, err)
		}
		if !strings.Contains(got, fixture.want) {
			t.Fatalf("%s: output %q misses marker %q", fixture.file, got, fixture.want)
		}
		if fixture.noCarrier == strings.Contains(got, "anuyabi") {
			t.Fatalf("%s: carrier prelude presence = %v, want %v", fixture.file, !fixture.noCarrier, fixture.noCarrier)
		}
	}
}
