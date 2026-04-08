package tests

import (
	"fmt"
	"langspec/dsl/editor"
	"os"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

const testSublimeSyntaxFile = "libs/langspec/examples/lspec.sublime-syntax"

func TestBuildSublimeSyntaxForDSL(t *testing.T) {
	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	if err := editor.BuildSublimeSyntaxForDSL(compiler, testSublimeSyntaxFile); err != nil {
		t.Fatalf("Sublime Syntax generation failed with error: %s", err.Error())
	}

	if err := assertSublimeContextReferencesExist(testSublimeSyntaxFile); err != nil {
		t.Fatalf("invalid sublime context graph: %v", err)
	}
}

type sublimeSyntaxFixture struct {
	Contexts map[string][]map[string]any `yaml:"contexts"`
}

func assertSublimeContextReferencesExist(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read syntax file: %w", err)
	}
	normalizedContent := normalizeYAMLForGoParser(content)

	var syntax sublimeSyntaxFixture
	if err := yaml.Unmarshal(normalizedContent, &syntax); err != nil {
		return fmt.Errorf("parse syntax yaml: %w", err)
	}

	for ctxName, entries := range syntax.Contexts {
		for _, entry := range entries {
			for _, target := range collectContextReferences(entry) {
				if _, ok := syntax.Contexts[target]; ok {
					continue
				}
				return fmt.Errorf("context %q references missing context %q", ctxName, target)
			}
		}
	}
	return nil
}

func normalizeYAMLForGoParser(content []byte) []byte {
	// go.yaml.in/yaml/v4 rejects %YAML 1.2 directives as "incompatible YAML document".
	// This test only validates context-graph wiring and does not depend on directives.
	if len(content) == 0 {
		return content
	}
	lines := strings.Split(string(content), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "%YAML ") {
			continue
		}
		out = append(out, line)
	}
	return []byte(strings.Join(out, "\n"))
}

func collectContextReferences(entry map[string]any) []string {
	var out []string
	out = append(out, collectContextReferencesFromValue(entry["push"])...)
	out = append(out, collectContextReferencesFromValue(entry["set"])...)
	out = append(out, collectContextReferencesFromValue(entry["include"])...)
	return out
}

func collectContextReferencesFromValue(value any) []string {
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, raw := range v {
			name, ok := raw.(string)
			if !ok || name == "" {
				continue
			}
			out = append(out, name)
		}
		return out
	default:
		return nil
	}
}
