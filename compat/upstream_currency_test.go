package compat

import (
	_ "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"testing"
)

// These symbols identify the additive contracts consumed by current Bloem hosts.
func TestUpstreamV023WireAdditions(t *testing.T) {
	for _, name := range []protoreflect.FullName{
		"silo.plugin.v1.AuthProviderChecks",
		"silo.plugin.v1.NetworkIdentityAuth",
		"silo.plugin.v1.RequestDescriptor.seasons",
		"silo.plugin.v1.TargetStatus.progress",
		"silo.plugin.v1.TargetStatus.wording",
		"silo.plugin.v1.WatchSyncProviderDescriptor.import_ratings",
		"silo.plugin.v1.WatchSyncProviderDescriptor.sync_dropped",
		"silo.plugin.v1.WatchSyncProviderDescriptor.rating_export_requires_watched",
	} {
		if _, err := protoregistry.GlobalFiles.FindDescriptorByName(name); err != nil {
			t.Errorf("current host contract missing %s: %v", name, err)
		}
	}
}
