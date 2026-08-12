package projectidentity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVondelModuleAndAttribution(t *testing.T) {
	root := filepath.Join("..", "..")
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(goMod), "module github.com/Vondel-Media/vondel-plugin-sdk\n") {
		t.Fatalf("unexpected module declaration: %s", goMod)
	}
	notice, err := os.ReadFile(filepath.Join(root, "NOTICE"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"Silo Plugin SDK",
		"v0.13.2",
		"1ad0fe54408e99d35e6aee86c489a0edd528f6b2",
		"Apache-2.0",
		"not affiliated with or endorsed by Silo Media L.L.C.",
	} {
		if !strings.Contains(string(notice), required) {
			t.Errorf("NOTICE missing %q", required)
		}
	}
}
