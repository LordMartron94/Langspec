package toolchain

import (
	"fmt"
	"io/fs"
	"langspec/dsl"
)

const (
	TmCommentsScopeKey          = "scope"
	TmCommentsScopeExtensionKey = "scope-extension"
	TmCommentsSingleLineKey     = "single-line-comment-start"
	TmCommentsBlockStartKey     = "block-comment-start"
	TmCommentsBlockEndKey       = "block-comment-end"
)

/*
TMCommentsConfigurationFromPragma builds TMCommentsConfiguration from tool.tm_comments PRAGMA settings.

Requires exactly one of scope or scope-extension (scope-extension is combined as "source" + extension,
matching Sublime BaseScope). Requires single-line-comment-start. Block comment start/end must both be
set or both omitted.
*/
func TMCommentsConfigurationFromPragma(pragma dsl.ToolPragma) (TMCommentsConfiguration, error) {
	var cfg TMCommentsConfiguration
	st := pragma.Settings

	scope, scopeOK := pragmaSettingString(st, TmCommentsScopeKey)
	ext, extOK := pragmaSettingString(st, TmCommentsScopeExtensionKey)
	switch {
	case scopeOK && extOK:
		return cfg, fmt.Errorf("tm comments: set only one of %q and %q", TmCommentsScopeKey, TmCommentsScopeExtensionKey)
	case scopeOK:
		cfg.Scope = scope
	case extOK:
		cfg.Scope = fmt.Sprintf("source%s", ext)
	default:
		return cfg, fmt.Errorf("tm comments: %q or %q is required", TmCommentsScopeKey, TmCommentsScopeExtensionKey)
	}

	single, ok := pragmaSettingString(st, TmCommentsSingleLineKey)
	if !ok {
		return cfg, fmt.Errorf("tm comments: %q is required", TmCommentsSingleLineKey)
	}
	cfg.SingleLineCommentStart = single

	bs, bsOK := pragmaSettingString(st, TmCommentsBlockStartKey)
	be, beOK := pragmaSettingString(st, TmCommentsBlockEndKey)
	switch {
	case bsOK && beOK:
		blockStart := bs
		blockEnd := be
		cfg.BlockCommentStart = &blockStart
		cfg.BlockCommentEnd = &blockEnd
	case !bsOK && !beOK:
		break
	default:
		return cfg, fmt.Errorf("tm comments: %q and %q must both be set or both omitted", TmCommentsBlockStartKey, TmCommentsBlockEndKey)
	}

	return cfg, nil
}

func pragmaSettingString(settings map[string]any, key string) (string, bool) {
	v, ok := settings[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return "", false
	}
	return s, true
}

func readTmCommentsTemplate(name string) ([]byte, error) {
	return fs.ReadFile(tmCommentsTemplates, name)
}
