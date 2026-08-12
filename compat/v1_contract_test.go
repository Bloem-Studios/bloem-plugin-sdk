package compat

import (
	"testing"

	pluginv1 "github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/capability"
	"github.com/Vondel-Media/vondel-plugin-sdk/pkg/pluginsdk/runtime"
	"google.golang.org/protobuf/reflect/protoreflect"
)

func TestV1WireContract(t *testing.T) {
	file := pluginv1.File_silo_plugin_v1_common_proto
	if got := string(file.Package()); got != "silo.plugin.v1" {
		t.Fatalf("protobuf package = %q", got)
	}
	manifest := file.Messages().ByName("PluginManifest")
	if manifest == nil {
		t.Fatal("PluginManifest descriptor missing")
	}
	field := manifest.Fields().ByName("silo_api_version")
	if field == nil || field.Number() != protoreflect.FieldNumber(4) {
		t.Fatalf("silo_api_version field = %#v", field)
	}

	services := []struct {
		file protoreflect.FileDescriptor
		name protoreflect.Name
	}{
		{pluginv1.File_silo_plugin_v1_common_proto, "Runtime"},
		{pluginv1.File_silo_plugin_v1_metadata_provider_proto, "MetadataProvider"},
		{pluginv1.File_silo_plugin_v1_scan_source_proto, "ScanSource"},
		{pluginv1.File_silo_plugin_v1_runtime_host_proto, "RuntimeHost"},
	}
	for _, service := range services {
		desc := service.file.Services().ByName(service.name)
		if desc == nil || string(desc.FullName()) != "silo.plugin.v1."+string(service.name) {
			t.Errorf("service %s changed: %#v", service.name, desc)
		}
	}

	if runtime.MagicCookieKey != "SILO_PLUGIN" ||
		runtime.MagicCookieValue != "silo-rpc-plugin-v1" ||
		runtime.PluginSetName != "silo" || runtime.ProtocolVersion != 1 {
		t.Fatal("go-plugin handshake contract changed")
	}

	for _, required := range []string{
		"metadata_provider.v1",
		"image_resolver.v1",
		"scan_source.v1",
	} {
		found := false
		for _, actual := range capability.KnownTypes {
			found = found || actual == required
		}
		if !found {
			t.Errorf("capability %q missing", required)
		}
	}
}
