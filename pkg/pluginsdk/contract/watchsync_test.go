package contract_test

import (
	v1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"testing"
)

// Host and adapter must agree on published numeric IDs without requiring a
// plugin to register two copies of the public silo.plugin.v1 descriptors.
func TestWatchSyncMatchesHostStateContract(t *testing.T) {
	d := (&v1.WatchSyncProviderDescriptor{}).ProtoReflect().Descriptor()
	for name, number := range map[protoreflect.Name]protoreflect.FieldNumber{"scrobble_playback": 16, "import_ratings": 17, "export_ratings": 18, "sync_dropped": 19} {
		field := d.Fields().ByName(name)
		if field == nil || field.Number() != number {
			t.Errorf("host field %s must retain number %d", name, number)
		}
	}
	op := v1.WatchSyncOperation(0).Descriptor()
	for name, number := range map[protoreflect.Name]protoreflect.EnumNumber{"WATCH_SYNC_OPERATION_MARK_WATCHED": 1, "WATCH_SYNC_OPERATION_SCROBBLE_STOP": 9, "WATCH_SYNC_OPERATION_SET_RATING": 10, "WATCH_SYNC_OPERATION_REMOVE_RATING": 11} {
		value := op.Values().ByName(name)
		if value == nil || value.Number() != number {
			t.Errorf("host operation %s must retain number %d", name, number)
		}
	}
}
