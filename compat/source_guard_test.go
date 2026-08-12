package compat

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtoSourcesPreserveWireIdentity(t *testing.T) {
	root := filepath.Join("..", "proto")
	inspected := 0
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".proto" {
			return nil
		}
		inspected++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		for _, forbidden := range []string{"package vondel.plugin", "vondel_api_version"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains forbidden wire rename %q", path, forbidden)
			}
		}
		const goPackage = "option go_package = \"github.com/Vondel-Media/vondel-plugin-sdk/"
		if !strings.Contains(text, goPackage) {
			t.Errorf("%s does not use the Vondel build-time Go package", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if inspected == 0 {
		t.Fatal("no protobuf sources inspected")
	}
}
