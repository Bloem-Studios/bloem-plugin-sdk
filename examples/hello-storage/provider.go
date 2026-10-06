package main

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"strconv"

	storagev1 "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/bloem/plugin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type provider struct {
	storagev1.UnimplementedStorageProviderServer
	book []byte
}

func newProvider() (*provider, error) {
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, entry := range []struct {
		name string
		data []byte
	}{
		{"mimetype", []byte("application/epub+zip")},
		{"META-INF/container.xml", []byte("<container><rootfile full-path=\"book.opf\"/></container>")},
		{"book.opf", []byte(`<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="id">fixture</dc:identifier><dc:title>Storage Fixture</dc:title><dc:language>en</dc:language></metadata><manifest><item id="body" href="OEBPS/body.xhtml" media-type="application/xhtml+xml"/></manifest><spine><itemref idref="body"/></spine></package>`)},
		{"OEBPS/body.xhtml", []byte(`<html xmlns="http://www.w3.org/1999/xhtml"><head><title>Fixture</title></head><body><p>Native storage fixture.</p></body></html>`)},
		{"OEBPS/payload.bin", bytes.Repeat([]byte("x"), 5<<20)},
	} {
		w, err := z.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			return nil, err
		}
		if _, err = w.Write(entry.data); err != nil {
			return nil, err
		}
	}
	if err := z.Close(); err != nil {
		return nil, err
	}
	return &provider{book: b.Bytes()}, nil
}

func (p *provider) Describe(ctx context.Context, _ *storagev1.DescribeRequest) (*storagev1.DescribeResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	// Synthetic sentinel only: proves the conformance launcher excludes parent env.
	if _, present := os.LookupEnv("BLOEM_STORAGE_TEST_SENTINEL"); present {
		return nil, status.Error(codes.PermissionDenied, "parent environment inherited")
	}
	return &storagev1.DescribeResponse{Revision: 1, Sources: []*storagev1.Source{{Id: "fixture", Name: "Synthetic ebooks", RootEntryId: "root", RevisionPinnedReads: true}}}, nil
}
func (p *provider) entry(source, id string) (*storagev1.Entry, error) {
	if source != "fixture" {
		return nil, status.Error(codes.NotFound, "source absent")
	}
	e := &storagev1.Entry{Id: id, Name: id + ".epub", LogicalPath: id + ".epub", Kind: storagev1.EntryKind_ENTRY_KIND_FILE, Size: int64(len(p.book)), Revision: "v1"}
	switch id {
	case "book", "revision-conflict", "duplicate", "short", "blocking":
	case "root", "chapters":
		e.Name = id
		e.LogicalPath = id
		e.Kind = storagev1.EntryKind_ENTRY_KIND_DIRECTORY
		e.Size = 0
	default:
		return nil, status.Error(codes.NotFound, "entry absent")
	}
	return e, nil
}
func (p *provider) Stat(ctx context.Context, r *storagev1.StatRequest) (*storagev1.StatResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	if r.GetExpectedRevision() == "" {
		return nil, status.Error(codes.InvalidArgument, "expected revision required")
	}
	e, err := p.entry(r.GetSourceId(), r.GetEntryId())
	if err != nil {
		return nil, err
	}
	if r.GetExpectedRevision() != e.GetRevision() {
		return nil, status.Error(codes.FailedPrecondition, "revision changed")
	}
	return &storagev1.StatResponse{Entry: e}, nil
}
func (p *provider) List(ctx context.Context, r *storagev1.ListRequest) (*storagev1.ListResponse, error) {
	if err := ctx.Err(); err != nil {
		return nil, status.FromContextError(err).Err()
	}
	dir, err := p.entry(r.GetSourceId(), r.GetDirectoryId())
	if err != nil {
		return nil, err
	}
	if dir.GetKind() != storagev1.EntryKind_ENTRY_KIND_DIRECTORY {
		return nil, status.Error(codes.InvalidArgument, "not a directory")
	}
	ids := []string{"book", "chapters"}
	if r.GetDirectoryId() == "chapters" {
		ids = nil
	}
	start := 0
	if r.GetCursor() != "" {
		start, err = strconv.Atoi(r.GetCursor())
		if err != nil || start < 0 || start > len(ids) {
			return nil, status.Error(codes.InvalidArgument, "invalid cursor")
		}
	}
	limit := int(r.GetMaxEntries())
	if limit == 0 || limit > 512 {
		limit = 512
	}
	end := start + limit
	if end > len(ids) {
		end = len(ids)
	}
	out := &storagev1.ListResponse{Complete: end == len(ids)}
	for _, id := range ids[start:end] {
		e, err := p.entry("fixture", id)
		if err != nil {
			return nil, err
		}
		out.Entries = append(out.Entries, e)
	}
	if !out.Complete {
		out.NextCursor = strconv.Itoa(end)
	}
	return out, nil
}
func (p *provider) Read(r *storagev1.ReadRequest, stream storagev1.StorageProvider_ReadServer) error {
	entry, err := p.Stat(stream.Context(), &storagev1.StatRequest{SourceId: r.GetSourceId(), EntryId: r.GetEntryId(), ExpectedRevision: r.GetExpectedRevision()})
	if err != nil {
		return err
	}
	if entry.GetEntry().GetKind() != storagev1.EntryKind_ENTRY_KIND_FILE || r.GetOffset() < 0 || r.GetLength() < 0 || r.GetLength() > 8<<20 || r.GetOffset() > int64(len(p.book)) || r.GetLength() > int64(len(p.book))-r.GetOffset() {
		return status.Error(codes.InvalidArgument, "invalid file range")
	}
	if r.GetEntryId() == "revision-conflict" {
		return status.Error(codes.FailedPrecondition, "revision changed before read")
	}
	if r.GetEntryId() == "blocking" {
		if r.GetLength() > 0 {
			if err := stream.Send(&storagev1.ReadChunk{Offset: r.GetOffset(), Data: p.book[r.GetOffset() : r.GetOffset()+1]}); err != nil {
				return err
			}
		}
		<-stream.Context().Done()
		return status.FromContextError(stream.Context().Err()).Err()
	}
	end := r.GetOffset() + r.GetLength()
	if r.GetLength() == 0 {
		return stream.Send(&storagev1.ReadChunk{Offset: r.GetOffset(), Eof: true})
	}
	for offset := r.GetOffset(); offset < end; {
		next := offset + (128 << 10)
		if next > end {
			next = end
		}
		chunk := &storagev1.ReadChunk{Offset: offset, Data: p.book[offset:next], Eof: next == end}
		if r.GetEntryId() == "short" {
			chunk.Data = chunk.Data[:len(chunk.Data)-1]
			chunk.Eof = true
		}
		if err := stream.Send(chunk); err != nil {
			return err
		}
		if r.GetEntryId() == "duplicate" {
			return stream.Send(chunk)
		}
		if r.GetEntryId() == "short" {
			return nil
		}
		offset = next
	}
	return nil
}
