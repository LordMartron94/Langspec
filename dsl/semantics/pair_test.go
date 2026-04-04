package semantics

import "testing"

func TestPairNameFromTokPairReferenceRaw(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"@Foo", "Foo"},
		{"@FooBar", "FooBar"},
		{" @Pair1 ", "Pair1"},
		{"", ""},
		{"Foo", ""},
		{"@", ""},
		{"@ ", ""},
	}
	for _, tt := range tests {
		got := PairNameFromTokPairReferenceRaw([]byte(tt.raw))
		if got != tt.want {
			t.Errorf("PairNameFromTokPairReferenceRaw(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}
