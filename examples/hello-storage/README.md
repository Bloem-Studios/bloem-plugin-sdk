# Synthetic native-storage fixture

This executable exercises the [native-storage contract](../../docs/storage-provider.md).
It creates an EPUB in memory and never connects to a storage backend. It is a
conformance fixture, not a production storage plugin or catalog package. It has
no backend client, credential schema or `WithConfigure` callback. To create a real
provider, follow the [published SDK authoring and configuration recipe](../../docs/storage-provider.md#create-a-provider-with-the-published-sdk)
and implement the backend operations; do not copy its intentional fault behavior.
Native installation uses [approved raw binaries](../../docs/storage-provider.md#package-approval-and-native-installation),
not the ordinary plugin ZIP upload.

Commands assume the repository root is the working directory:

```sh
GOWORK=off go test ./examples/hello-storage ./pkg/pluginsdk/runtime ./compat
mkdir -p dist
GOWORK=off go build -trimpath -o dist/hello-storage ./examples/hello-storage
./dist/hello-storage manifest
```

The manifest command reports the executable's real SHA-256. The host must launch
normal service mode using the established plugin handshake; running the binary
directly does not start an unauthenticated HTTP server.

`Describe` advertises `fixture` and `scale` sources. The first contains `book`
and a `chapters` directory; the second generates two million virtual ebook
identities page by page, without allocating the full namespace. Both advertise
revision-pinned reads of the synthetic `v1` content.

Tests can directly address intentional fault entries `revision-conflict`,
`duplicate`, `short` and `blocking`. The unadvertised `scale-failure` test source
fails after its first page. These faults check host rejection/cancellation;
they are not examples of valid production responses. A synthetic environment
sentinel checks that a conformance launcher excludes unrelated parent variables.

A successful fixture run proves protocol behavior only. It does not admit an S3
backend, authorize library access or measure real catalog throughput.
