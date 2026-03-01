package dsl

import (
	"io"
	"os"
)

/*
LangSpecDiagnosticSink directs compilation diagnostics (syntax errors, parse trace,
validation results, LST dumps) to an io.Writer. When Writer is nil, no diagnostic
output is produced. Use DefaultLangSpecDiagnosticSink for stdout, or pass a
bytes.Buffer in tests to capture output.
*/
type LangSpecDiagnosticSink struct {
	Writer io.Writer
}

/*
DefaultLangSpecDiagnosticSink returns a sink that writes to os.Stdout.
Use when the caller wants human-readable diagnostics on the standard output.
*/
func DefaultLangSpecDiagnosticSink() *LangSpecDiagnosticSink {
	return &LangSpecDiagnosticSink{Writer: os.Stdout}
}
