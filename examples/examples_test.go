package examples

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestExampleManifestIdentity(t *testing.T) {
	tests := []struct {
		path string
		id   string
	}{
		{"hello-scheduled-task/manifest.json", "bloem.example.hello-task"},
		{"hello-runtime-host/manifest.json", "bloem.example.runtime-host"},
	}
	for _, tc := range tests {
		t.Run(filepath.Base(filepath.Dir(tc.path)), func(t *testing.T) {
			data, err := os.ReadFile(tc.path)
			if err != nil {
				t.Fatal(err)
			}
			m, err := manifest.Load(data)
			if err != nil {
				t.Fatal(err)
			}
			if got := m.GetPluginId(); got != tc.id {
				t.Fatalf("plugin_id = %q, want %q", got, tc.id)
			}
			if got := m.GetSiloApiVersion(); got != "v1" {
				t.Fatalf("silo_api_version = %q", got)
			}
		})
	}
}
