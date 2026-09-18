package harness

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anuy-lang/anuy/experimental/parser"
)

func TestParserCoreAcceptFixturesParse(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "fixtures", "parser-core", "accept", "*.anuy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no parser-core accept fixtures found")
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(string(source)); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

func TestParserCoreRejectFixturesFail(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "fixtures", "parser-core", "reject", "*.anuy"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no parser-core reject fixtures found")
	}
	for _, path := range paths {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parser.Parse(string(source)); err == nil {
			t.Fatalf("%s: reject fixture accepted", path)
		}
	}
}
