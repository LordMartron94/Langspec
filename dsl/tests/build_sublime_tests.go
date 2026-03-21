package tests

import (
	"langspec/dsl/editor"
	"testing"
)

const testSublimeSyntaxFile = "libs/langspec/examples/lspec.sublime-syntax"

func TestBuildSublimeSyntaxForDSL(t *testing.T) {
	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	if err := editor.BuildSublimeSyntaxForDSL(compiler, testSublimeSyntaxFile); err != nil {
		t.Fatalf("Sublime Syntax generation failed with error: %s", err.Error())
	}
}
