package toolchain

import "embed"

//go:embed comments-single.template.tmPreferences comments-multiple.template.tmPreferences
var tmCommentsTemplates embed.FS
