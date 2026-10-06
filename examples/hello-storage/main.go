// Command hello-storage is an executable native-storage conformance fixture.
// It owns a synthetic ebook and fault entries; it accesses no real backend.
package main

import (
	_ "embed"

	sdkruntime "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginsdk/runtime"
)

//go:embed manifest.json
var manifestJSON []byte

func main() {
	provider, err := newProvider()
	if err != nil {
		panic(err)
	}
	sdkruntime.ServeManifestWithOptions(manifestJSON, "0.1.0", sdkruntime.CapabilityServers{}, sdkruntime.WithStorageProvider(provider))
}
