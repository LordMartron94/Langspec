package sublime

import (
	"testing"

	"langspec/editor"
)

func TestIncludePrototypeForStateUsesLocalContextAndTransitions(t *testing.T) {
	falseVal := false
	config := ExtractionConfig[string]{
		ExtractIncludePrototype: func(ctx string) *bool {
			switch ctx {
			case "string-body", "string-token":
				return &falseVal
			case "main", "keyword-token":
				return nil
			default:
				return nil
			}
		},
	}

	t.Run("state context", func(t *testing.T) {
		state := editor.EditorState[string, string]{
			Context: "string-body",
		}
		got := includePrototypeForState(state, config)
		if got == nil || *got {
			t.Fatalf("IncludePrototype = %v, want false", got)
		}
	})

	t.Run("transition match context", func(t *testing.T) {
		state := editor.EditorState[string, string]{
			Context: "main",
			Transitions: []editor.EditorTransition[string, string]{
				{MatchContext: "keyword-token"},
			},
		}
		got := includePrototypeForState(state, config)
		if got != nil {
			t.Fatalf("IncludePrototype = %v, want nil", *got)
		}
	})

	t.Run("transition disables when manifest says so", func(t *testing.T) {
		state := editor.EditorState[string, string]{
			Context: "main",
			Transitions: []editor.EditorTransition[string, string]{
				{MatchContext: "string-token"},
			},
		}
		got := includePrototypeForState(state, config)
		if got == nil || *got {
			t.Fatalf("IncludePrototype = %v, want false", got)
		}
	})
}
