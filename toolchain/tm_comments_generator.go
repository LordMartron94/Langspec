package toolchain

import (
	"errors"
	"fmt"
	"foundation/system"
	"langspec/dsl"
	"strings"
)

const (
	TmCommentsToolName      = "tm_comments"
	TmCommentsOutputPathKey = "output-path"
	TmCommentsEnabledKey    = "enable"
)

type TMCommentsConfiguration struct {
	SingleLineCommentStart string
	BlockCommentStart      *string
	BlockCommentEnd        *string
	Scope                  string
}

func RunTMCommentsToolchain[TNodeKind ~uint32](compileResult *dsl.LangSpecCompileResult[TNodeKind], cfg TMCommentsConfiguration) error {
	if cfg.BlockCommentStart != nil && cfg.BlockCommentEnd == nil {
		return fmt.Errorf("if block comment start is given, block comment end must be set as well")
	}

	if cfg.BlockCommentStart == nil && cfg.BlockCommentEnd != nil {
		return fmt.Errorf("if block comment end is given, block comment start must be set as well")
	}

	if compileResult == nil {
		return nil
	}

	pragma, ok := compileResult.CompiledToolPragmas[TmCommentsToolName]

	if !ok {
		return nil
	}

	enabled, ok := pragma.Settings[TmCommentsEnabledKey].(string)
	if !ok {
		return fmt.Errorf("tm comments toolchain: missing or invalid '%s'", TmCommentsEnabledKey)
	}

	if enabled == "false" {
		return nil
	}
	if enabled != "true" {
		return fmt.Errorf("unexpected enabled setting value: '%s'", enabled)
	}

	outputPaths, err := ExtractOutputPathsFromPragma(pragma, TmCommentsOutputPathKey)
	if err != nil {
		return err
	}

	var outputError error

	for _, path := range outputPaths {
		if err := generateTmCommentsFile(path, cfg); err != nil {
			outputError = errors.Join(outputError, err)
		}
	}

	return outputError
}

func generateTmCommentsFile(path string, cfg TMCommentsConfiguration) error {
	var content string

	if cfg.BlockCommentStart != nil {
		raw, err := readTmCommentsTemplate("comments-multiple.template.tmPreferences")
		if err != nil {
			return fmt.Errorf("could not read embedded template: %w", err)
		}
		content = string(raw)
		content = applyMultiComment(content, *cfg.BlockCommentStart, *cfg.BlockCommentEnd)
	} else {
		raw, err := readTmCommentsTemplate("comments-single.template.tmPreferences")
		if err != nil {
			return fmt.Errorf("could not read embedded template: %w", err)
		}
		content = string(raw)
	}

	content = applyScope(content, cfg.Scope)
	content = applySingleComment(content, cfg.SingleLineCommentStart)

	if err := system.FileWriteString(path, content); err != nil {
		return fmt.Errorf("could not write output: %w", err)
	}

	return nil
}

func applyScope(content string, scope string) string {
	return strings.ReplaceAll(content, "${SCOPE}", scope)
}

func applySingleComment(content string, singleLineCommentStart string) string {
	return strings.ReplaceAll(content, "${SINGLE_LINE_COMMENT_START}", singleLineCommentStart)
}

func applyMultiComment(content string, multiCommentStart, multiCommentEnd string) string {
	out := strings.ReplaceAll(content, "${BLOCK_COMMENT_START}", multiCommentStart)
	return strings.ReplaceAll(out, "${BLOCK_COMMENT_END}", multiCommentEnd)
}
