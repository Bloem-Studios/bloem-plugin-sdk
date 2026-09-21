package main

import (
	"context"
	_ "embed"

	pluginv1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1"
	sdkruntime "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/runtime"
)

const version = "0.13.2"

//go:embed manifest.json
var manifestJSON []byte

type metadataProvider struct {
	pluginv1.UnimplementedMetadataProviderServer
}

func (*metadataProvider) Search(context.Context, *pluginv1.SearchMetadataRequest) (*pluginv1.SearchMetadataResponse, error) {
	return &pluginv1.SearchMetadataResponse{}, nil
}

func main() {
	sdkruntime.ServeManifest(manifestJSON, version, sdkruntime.CapabilityServers{
		MetadataProvider: &metadataProvider{},
	})
}
