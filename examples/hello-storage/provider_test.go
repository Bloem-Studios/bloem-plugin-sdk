package main

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	storagev1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/bloem/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestFixtureContainsSeekableEbook(t *testing.T) {
	p, err := newProvider()
	if err != nil {
		t.Fatal(err)
	}
	r, err := p.Stat(context.Background(), &storagev1.StatRequest{SourceId: "fixture", EntryId: "book", ExpectedRevision: "v1"})
	if err != nil || r.GetEntry().GetSize() <= 4<<20 {
		t.Fatalf("fixture Stat: %v %v", r, err)
	}
	z, err := zip.NewReader(bytes.NewReader(p.book), int64(len(p.book)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := z.Open("mimetype")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil || string(b) != "application/epub+zip" {
		t.Fatalf("mimetype: %q %v", b, err)
	}
	for _, tc := range []struct {
		source, entry, revision string
		code                    codes.Code
	}{{"missing", "book", "v1", codes.NotFound}, {"fixture", "missing", "v1", codes.NotFound}, {"fixture", "book", "v2", codes.FailedPrecondition}, {"fixture", "book", "", codes.InvalidArgument}} {
		_, err := p.Stat(context.Background(), &storagev1.StatRequest{SourceId: tc.source, EntryId: tc.entry, ExpectedRevision: tc.revision})
		if status.Code(err) != tc.code {
			t.Fatalf("Stat status: %v want %v", err, tc.code)
		}
	}
}
