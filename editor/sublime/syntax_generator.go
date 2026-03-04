package sublime

import (
	"fmt"
	"foundation/system"
	"langspec/editor"
	"slices"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"
)

// SublimeTextGenerateSyntaxFile writes a Sublime Text YAML syntax definition to
// syntaxFile using the given editor IR. fileExtensions is the list of extensions
// (e.g. []string{".lspec"}) to associate with the language. Returns an error
// if metadata or rules cannot be serialized or if the file cannot be written.
func SublimeTextGenerateSyntaxFile(
	editorIR *editor.PushDownAutomatonIR,
	syntaxFile string,
	fileExtensions []string,
	forceDisableTargetStateValidation bool,
) error {
	sb := &strings.Builder{}

	writeHeader(sb, editorIR.LanguageName, editorIR.LanguageVersion)
	sb.WriteString("\n")

	if err := writeLanguageMetadata(sb, editorIR, fileExtensions); err != nil {
		return fmt.Errorf("could not construct language metadata section: %w", err)
	}

	sb.WriteString("\n")

	if err := writeRules(sb, editorIR, forceDisableTargetStateValidation); err != nil {
		return fmt.Errorf("could not construct rules section: %w", err)
	}

	sb.WriteString("\n")

	if err := system.FileWriteString(syntaxFile, sb.String()); err != nil {
		return fmt.Errorf("could not write syntax file: %w", err)
	}

	return nil
}

const headerLine = "# ======================================================\n"

const timeFormat = "2006-01-02 15:04:05 MST"

type languageMetadata struct {
	Name           string   `yaml:"name"`
	FileExtensions []string `yaml:"file_extensions"`
	Scope          string   `yaml:"scope"`
	Version        int      `yaml:"version"`
}

func writeLanguageMetadata(sb *strings.Builder, editorIR *editor.PushDownAutomatonIR, fileExtensions []string) error {
	writeSectionHeader(sb, "Language Metadata")

	if value, err := yaml.Marshal(languageMetadata{
		Name:           editorIR.LanguageName,
		FileExtensions: fileExtensions,
		Scope:          "source" + editorIR.ScopeExtension,
		Version:        2,
	}); err != nil {
		return fmt.Errorf("error marshaling language metadata: %w", err)
	} else {
		sb.Write(value)
	}

	return nil
}

type contextEntry struct {
	MetaScope      *string        `yaml:"meta_scope,omitempty"`
	Match          *string        `yaml:"match,omitempty"`
	Scope          string         `yaml:"scope,omitempty"`
	Push           *string        `yaml:"push,omitempty"`
	Set            *string        `yaml:"set,omitempty"`
	Pop            any            `yaml:"pop,omitempty"`
	Include        *string        `yaml:"include,omitempty"`
	Captures       map[int]string `yaml:"captures,omitempty"`
	Embed          *string        `yaml:"embed,omitempty"`
	EmbedScope     *string        `yaml:"embed_scope,omitempty"`
	Escape         *string        `yaml:"escape,omitempty"`
	EscapeCaptures map[int]string `yaml:"escape_captures,omitempty"`
}

type contextsSection struct {
	Contexts map[string][]contextEntry `yaml:"contexts"`
}

func writeRules(sb *strings.Builder, editorIR *editor.PushDownAutomatonIR, disableError bool) error {
	writeSectionHeader(sb, "Contexts & Rules")

	idToLabel := make(map[editor.StateID]string)
	for _, state := range editorIR.States {
		idToLabel[state.ID] = state.Label
	}

	contexts := make(map[string][]contextEntry)
	var rootIncludes []contextEntry

	for _, state := range editorIR.States {
		var entries []contextEntry

		// 1. Meta Scope (Must be first)
		if state.MetaScope != "" {
			entries = append(entries, contextEntry{MetaScope: &state.MetaScope})
		}

		// 2. Rules with priority < PriorityAfterIncludes (ordered by Priority)
		// 3. Includes
		// 4. Rules with priority >= PriorityAfterIncludes (e.g. fallback)
		rules := slices.Clone(state.Rules)
		slices.SortFunc(rules, func(a, b editor.StateRule) int { return a.Priority - b.Priority })
		for _, rule := range rules {
			if rule.Priority < editor.PriorityAfterIncludes {
				entries = append(entries, convertRuleToEntry(rule, idToLabel))
			}
		}
		for _, incID := range state.Includes {
			targetLabel, ok := idToLabel[incID]
			if ok {
				labelCopy := targetLabel
				entries = append(entries, contextEntry{Include: &labelCopy})
			} else if !disableError {
				return fmt.Errorf("missing target ID %d for include in state %s", incID, state.Label)
			}

		}
		for _, rule := range rules {
			if rule.Priority >= editor.PriorityAfterIncludes {
				entries = append(entries, convertRuleToEntry(rule, idToLabel))
			}
		}

		contexts[state.Label] = entries

		if state.IsRootContext {
			labelCopy := state.Label
			rootIncludes = append(rootIncludes, contextEntry{Include: &labelCopy})
		}
	}

	// If the editor did not inject a "main" state (e.g. legacy IR), build main from root includes only.
	if _, ok := contexts["main"]; !ok {
		contexts["main"] = rootIncludes
	}

	if value, err := yaml.Marshal(contextsSection{Contexts: contexts}); err != nil {
		return fmt.Errorf("error marshaling context: %w", err)
	} else {
		sb.Write(value)
	}

	return nil
}

func convertRuleToEntry(rule editor.StateRule, idToLabel map[editor.StateID]string) contextEntry {
	matchStr := rule.RegEx
	entry := contextEntry{
		Match: &matchStr,
	}

	if rule.Scope != "" {
		entry.Scope = rule.Scope
	}

	if len(rule.Captures) > 0 {
		entry.Captures = rule.Captures
	}

	switch rule.Action {
	case editor.ACTION_PUSH:
		target := idToLabel[rule.ActionTarget]
		entry.Push = &target
	case editor.ACTION_SET:
		target := idToLabel[rule.ActionTarget]
		entry.Set = &target
	case editor.ACTION_POP:
		if rule.PopCount > 1 {
			entry.Pop = rule.PopCount
		} else {
			t := true
			entry.Pop = &t
		}
	case editor.ACTION_EMBED:
		if rule.Embed != "" {
			entry.Embed = &rule.Embed
		}
		if rule.EmbedScope != "" {
			entry.EmbedScope = &rule.EmbedScope
		}
		if rule.Escape != "" {
			entry.Escape = &rule.Escape
		}
		if len(rule.EscapeCaptures) > 0 {
			entry.EscapeCaptures = rule.EscapeCaptures
		}
	}
	return entry
}

func writeHeader(sb *strings.Builder, languageName, languageVersion string) {
	currentTime := time.Now()
	timeString := currentTime.Format(timeFormat)

	sb.WriteString("%YAML 1.2\n---\n\n")

	sb.WriteString(headerLine)
	sb.WriteString("#  GENERATED BY LangSpec -- DO NOT EDIT MANUALLY\n")
	sb.WriteString(headerLine)
	fmt.Fprintf(sb, "#  DATE: %s\n", timeString)
	fmt.Fprintf(sb, "#  GENERATED FOR: %s @ %s\n", languageName, languageVersion)
	sb.WriteString(headerLine)
}

func writeSectionHeader(sb *strings.Builder, headerContent string) {
	sb.WriteString(headerLine)
	fmt.Fprintf(sb, "#  %s\n", headerContent)
	sb.WriteString(headerLine)
}
