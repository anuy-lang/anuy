package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSemanticCoreGoldenFixtureIsDeterministic(t *testing.T) {
	path := filepath.Join("..", "fixtures", "semantic-core", "basic.golden")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "semantic-core fixture v1\n"
	if string(got) != want {
		t.Fatalf("fixture = %q, want %q", got, want)
	}
}
