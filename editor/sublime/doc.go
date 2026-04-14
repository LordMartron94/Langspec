// Package sublime integrates LangSpec with Sublime Text by auto-generating
// Sublime Text syntax definition files from the editor IR. The main entry point
// is GenerateSyntaxFile: it takes the editor IR, output path, file extensions,
// scope extraction config, optional prune/generation warning writers, and optional
// PeekEmitConfig. Grammar peek constraints live on EditorTransition.PeekAfterMatch;
// this package composes (?=...) lookaheads from lexing rules—regex assembly is not
// done in the editor IR layer.
package sublime
