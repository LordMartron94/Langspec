// Package sublime integrates LangSpec with Sublime Text by auto-generating
// Sublime Text syntax definition files from the editor IR. The main entrypoint
// is SublimeTextGenerateSyntaxFile: it takes a PushDownAutomatonIR, an output
// file path, and file extensions, and writes a YAML syntax definition that
// Sublime Text can load for highlighting.
package sublime
