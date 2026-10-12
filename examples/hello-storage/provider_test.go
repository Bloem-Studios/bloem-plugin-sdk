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

func TestPagedScaleNamespace(t *testing.T) {
	p, err := newProvider()
	if err != nil {
		t.Fatal(err)
	}
	page, err := p.List(context.Background(), &storagev1.ListRequest{SourceId: "scale", DirectoryId: "root", Cursor: "1999872", MaxEntries: 128})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.GetEntries()) != 128 || !page.GetComplete() || page.GetNextCursor() != "" || page.GetEntries()[0].GetId() != "book-1999872" || page.GetEntries()[127].GetId() != "book-1999999" {
		t.Fatalf("last scale page wrong: entries=%d complete=%v", len(page.GetEntries()), page.GetComplete())
	}
	_, err = p.List(context.Background(), &storagev1.ListRequest{SourceId: "scale-failure", DirectoryId: "root", Cursor: "512", MaxEntries: 512})
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("failure was not classified: %v", err)
	}
}
