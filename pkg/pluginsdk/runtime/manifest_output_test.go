package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/manifest"
)

// The host stores a request router's flags from the executable's own
// `manifest` output, so a flag declared in manifest.json must survive loading
// the embedded manifest and printing it again.
func TestManifestOutputKeepsRequestRouterOptionalFlags(t *testing.T) {
	embedded := []byte(`{
	  "plugin_id":"example.requests", "version":"1.0.0", "silo_api_version":"v1",
	  "capabilities":[{
	    "type":"request_router.v1", "id":"books", "display_name":"Books",
	    "request_router":{"api_key_optional":true, "base_url_optional":true, "reports_download_progress":true}
	  }]
	}`)
	m, err := manifest.LoadWithChecksum(embedded, "")
	if err != nil {
		t.Fatalf("LoadWithChecksum: %v", err)
	}
	cfg, err := manifestServeConfig(m, CapabilityServers{})
	if err != nil {
		t.Fatalf("manifestServeConfig: %v", err)
	}

	var out bytes.Buffer
	if err := writeManifest(context.Background(), &out, cfg.Servers.Runtime); err != nil {
		t.Fatalf("writeManifest: %v", err)
	}

	// The printed form uses JSON names, as the host reads it.
	var raw struct {
		Capabilities []struct {
			RequestRouter map[string]any `json:"requestRouter"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if len(raw.Capabilities) != 1 {
		t.Fatalf("capabilities = %d, want 1\n%s", len(raw.Capabilities), out.String())
	}
	for _, key := range []string{"apiKeyOptional", "baseUrlOptional", "reportsDownloadProgress"} {
		if raw.Capabilities[0].RequestRouter[key] != true {
			t.Errorf("requestRouter.%s = %v, want true\n%s", key, raw.Capabilities[0].RequestRouter[key], out.String())
		}
	}

	// And it decodes back to the same descriptor.
	var printed pluginv1.PluginManifest
	if err := protojson.Unmarshal(out.Bytes(), &printed); err != nil {
		t.Fatalf("protojson decode output: %v", err)
	}
	router := printed.GetCapabilities()[0].GetRequestRouter()
	if !router.GetApiKeyOptional() || !router.GetBaseUrlOptional() || !router.GetReportsDownloadProgress() {
		t.Fatalf("round-tripped request router = %v, want every declared flag true", router)
	}
}
