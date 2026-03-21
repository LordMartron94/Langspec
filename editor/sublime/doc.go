// Package sublime integrates LangSpec with Sublime Text by auto-generating
// Sublime Text syntax definition files from the editor IR. The main entry point
// is GenerateSyntaxFile: it takes a PushDownAutomatonIR, observation/token/node
// type parameters, an output file path, file extensions, and scope metadata, and
// writes a YAML syntax definition that Sublime Text can load for highlighting.
package sublime
