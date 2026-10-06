package compat

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var goPackageOption = regexp.MustCompile(`(?m)^\s*option\s+go_package\s*=\s*"([^"]+)"\s*;`)

func TestGoPackageViolationsRejectActiveOutsideDeclarationDespiteApprovedComment(t *testing.T) {
	text := `syntax = "proto3";

// option go_package = "github.com/Bloem-Studios/bloem-plugin-sdk/pkg/pluginproto/silo/plugin/v1;pluginv1";
option go_package = "example.com/outside/plugin/v1;pluginv1";
`

	violations := goPackageViolations(text)
	if len(violations) == 0 {
		t.Fatal("active outside go_package declaration accepted because approved text appeared in a comment")
	}
	if !strings.Contains(violations[0], "example.com/outside/plugin/v1;pluginv1") {
		t.Fatalf("violations = %q, want active outside go_package value", violations)
	}
}

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
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, violation := range wireIdentityViolations(relative, text) {
			t.Errorf("%s %s", path, violation)
		}
		for _, violation := range goPackageViolations(text) {
			t.Errorf("%s %s", path, violation)
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

func goPackageViolations(text string) []string {
	const expectedPrefix = "github.com/Bloem-Studios/bloem-plugin-sdk/"
	declarations := goPackageDeclarations(text)
	if len(declarations) == 0 {
		return []string{"does not use the Bloem build-time Go package"}
	}

	var violations []string
	for _, declaration := range declarations {
		if !strings.HasPrefix(declaration, expectedPrefix) {
			violations = append(violations, fmt.Sprintf(
				"declares go_package %q outside %q",
				declaration,
				expectedPrefix,
			))
		}
	}
	return violations
}

func goPackageDeclarations(text string) []string {
	matches := goPackageOption.FindAllStringSubmatch(stripProtoComments(text), -1)
	declarations := make([]string, 0, len(matches))
	for _, match := range matches {
		declarations = append(declarations, match[1])
	}
	return declarations
}

func stripProtoComments(text string) string {
	var stripped strings.Builder
	stripped.Grow(len(text))

	inLineComment := false
	inBlockComment := false
	inString := false
	escaped := false
	for index := 0; index < len(text); index++ {
		current := text[index]
		if inLineComment {
			if current == '\n' {
				inLineComment = false
				stripped.WriteByte(current)
			} else {
				stripped.WriteByte(' ')
			}
			continue
		}
		if inBlockComment {
			if current == '*' && index+1 < len(text) && text[index+1] == '/' {
				stripped.WriteString("  ")
				index++
				inBlockComment = false
			} else if current == '\n' {
				stripped.WriteByte(current)
			} else {
				stripped.WriteByte(' ')
			}
			continue
		}
		if inString {
			stripped.WriteByte(current)
			if escaped {
				escaped = false
			} else if current == '\\' {
				escaped = true
			} else if current == '"' {
				inString = false
			}
			continue
		}

		switch {
		case current == '/' && index+1 < len(text) && text[index+1] == '/':
			stripped.WriteString("  ")
			index++
			inLineComment = true
		case current == '/' && index+1 < len(text) && text[index+1] == '*':
			stripped.WriteString("  ")
			index++
			inBlockComment = true
		default:
			stripped.WriteByte(current)
			inString = current == '"'
		}
	}

	return stripped.String()
}

// Preserve legacy wire identities while permitting an independent namespace.
func TestWireIdentityGuardDistinguishesLegacyAndPrivateSources(t *testing.T) {
	for _, tc := range []struct {
		path, text string
		want       int
	}{
		{"silo/plugin/v1/common.proto", "package bloem.plugin.v1;", 1},
		{"silo/plugin/v1/common.proto", "string bloem_api_version = 1;", 1},
		{"silo/plugin/v1/common.proto", "package silo.plugin.v1;", 0},
		{"bloem/plugin/v1/storage_provider.proto", "package bloem.plugin.v1;", 0},
	} {
		if got := len(wireIdentityViolations(tc.path, tc.text)); got != tc.want {
			t.Fatalf("%s: got %d violations, want %d", tc.path, got, tc.want)
		}
	}
}

func wireIdentityViolations(relativePath, text string) []string {
	if !strings.HasPrefix(filepath.ToSlash(relativePath), "silo/") {
		return nil
	}
	var violations []string
	for _, forbidden := range []string{"package bloem.plugin", "bloem_api_version"} {
		if strings.Contains(text, forbidden) {
			violations = append(violations, fmt.Sprintf("contains forbidden wire rename %q", forbidden))
		}
	}
	return violations
}
