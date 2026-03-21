package tests

import "testing"

func RunAllLangSpecTests(t *testing.T) {
	TestMaintainerGenerateLSpecExample(t)
	TestParseGeneratedLSpecViaBootstrap(t)
	TestBuildSublimeSyntaxForDSL(t)
	TestClientDSLToolchain(t)
}
