package runtime

import (
	"context"
	"net"
	"testing"
	"time"

	storagev1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/bloem/plugin/v1"
	pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type storageStub struct {
	storagev1.UnimplementedStorageProviderServer
}

func (storageStub) Describe(context.Context, *storagev1.DescribeRequest) (*storagev1.DescribeResponse, error) {
	return &storagev1.DescribeResponse{Revision: 1}, nil
}

type deviceStub struct {
	pluginv1.UnimplementedWatchSyncDeviceAuthorizationServiceServer
}

func TestStorageOptionPreservesLegacyRuntime(t *testing.T) { testStorageOptions(t, false, true) }
func TestStorageOptionCombinesWithDeviceAuthorization(t *testing.T) {
	testStorageOptions(t, true, true)
}
func TestStorageOptionAbsentKeepsLegacyRuntime(t *testing.T) { testStorageOptions(t, false, false) }

func testStorageOptions(t *testing.T, device, storage bool) {
	t.Helper()
	var options serveManifestOptions
	if storage {
		WithStorageProvider(storageStub{})(&options)
	}
	if device {
		WithWatchSyncDeviceAuthorization(deviceStub{})(&options)
	}
	set := manifestPluginSet(CapabilityServers{Runtime: &manifestRuntime{manifest: &pluginv1.PluginManifest{PluginId: "legacy.fixture"}}}, options)
	p := set[PluginSetName].(plugin.GRPCPlugin)
	srv := grpc.NewServer()
	if err := p.GRPCServer(nil, srv); err != nil {
		t.Fatal(err)
	}
	if _, ok := srv.GetServiceInfo()["silo.plugin.v1.WatchSyncDeviceAuthorizationService"]; ok != device {
		t.Fatal("device service registration changed")
	}
	listener := bufconn.Listen(1 << 20)
	go func() { _ = srv.Serve(listener) }()
	t.Cleanup(srv.Stop)
	t.Cleanup(func() { _ = listener.Close() })
	conn, err := grpc.NewClient("passthrough:///fixture", grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	dispensed, err := p.GRPCClient(ctx, nil, conn)
	if err != nil {
		t.Fatal(err)
	}
	client, ok := dispensed.(*Client)
	if !ok || client.Conn() != conn {
		t.Fatal("legacy client type or connection changed")
	}
	m, err := client.Runtime().GetManifest(ctx, &pluginv1.GetManifestRequest{})
	if err != nil || m.GetManifest().GetPluginId() != "legacy.fixture" {
		t.Fatalf("legacy manifest: %v, %v", m, err)
	}
	r, err := storagev1.NewStorageProviderClient(conn).Describe(ctx, &storagev1.DescribeRequest{})
	if !storage {
		if status.Code(err) != codes.Unimplemented {
			t.Fatalf("unexpected service: %v", err)
		}
		return
	}
	if err != nil || r.GetRevision() != 1 {
		t.Fatalf("storage Describe: %v, %v", r, err)
	}
}
