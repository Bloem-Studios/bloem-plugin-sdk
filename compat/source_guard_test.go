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
		for _, forbidden := range []string{"package bloem.plugin", "bloem_api_version"} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s contains forbidden wire rename %q", path, forbidden)
			}
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
