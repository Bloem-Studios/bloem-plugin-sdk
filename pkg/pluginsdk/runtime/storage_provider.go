package runtime

import (
	storagev1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/bloem/plugin/v1"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// WithStorageProvider adds native file access on the existing connection.
// It does not change the released capability structs or Silo enum values.
func WithStorageProvider(server storagev1.StorageProviderServer) ServeManifestOption {
	return func(options *serveManifestOptions) { options.storageProvider = server }
}

func manifestPluginSet(servers CapabilityServers, options serveManifestOptions) plugin.PluginSet {
	set := pluginSetWithOptionalServices(servers, optionalServices{
		watchSyncDeviceAuthorization: options.watchSyncDeviceAuthorization,
		authProviderChecks:           options.authProviderChecks,
		networkIdentityAuth:          options.networkIdentityAuth,
	})
	if options.storageProvider != nil {
		set[PluginSetName] = &grpcPluginWithStorage{
			Plugin:     set[PluginSetName],
			GRPCPlugin: set[PluginSetName].(plugin.GRPCPlugin),
			storage:    options.storageProvider,
		}
	}
	return set
}

type grpcPluginWithStorage struct {
	plugin.Plugin
	plugin.GRPCPlugin
	storage storagev1.StorageProviderServer
}

func (p *grpcPluginWithStorage) GRPCServer(broker *plugin.GRPCBroker, server *grpc.Server) error {
	if err := p.GRPCPlugin.GRPCServer(broker, server); err != nil {
		return err
	}
	storagev1.RegisterStorageProviderServer(server, p.storage)
	return nil
}
