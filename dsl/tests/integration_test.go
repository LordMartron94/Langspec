package tests

import "testing"

// TestBuildSublimeSyntaxForDSLRun bridges the test function defined in
// build_sublime_tests.go into the standard Go test runner.
func TestBuildSublimeSyntaxForDSLRun(t *testing.T) {
	TestBuildSublimeSyntaxForDSL(t)
}
