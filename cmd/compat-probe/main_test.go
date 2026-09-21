package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/manifest"
)

func TestMetadataProviderSearchReturnsEmptySuccess(t *testing.T) {
	response, err := (&metadataProvider{}).Search(context.Background(), &pluginv1.SearchMetadataRequest{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if response == nil {
		t.Fatal("Search returned a nil response")
	}
	if got := len(response.GetResults()); got != 0 {
		t.Fatalf("results = %d, want 0", got)
	}
}

func TestManifestSubcommand(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "compat-probe")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build probe: %v\n%s", err, output)
	}
	output, err := exec.Command(binary, "manifest").CombinedOutput()
	if err != nil {
		t.Fatalf("probe manifest: %v\n%s", err, output)
	}
	m, err := manifest.Load(output)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.GetPluginId(); got != "bloem.compat.probe" {
		t.Fatalf("plugin_id = %q", got)
	}
	if got := m.GetSiloApiVersion(); got != "v1" {
		t.Fatalf("silo_api_version = %q", got)
	}
	if len(m.GetCapabilities()) != 1 ||
		m.GetCapabilities()[0].GetType() != "metadata_provider.v1" {
		t.Fatalf("capabilities = %#v", m.GetCapabilities())
	}
}
