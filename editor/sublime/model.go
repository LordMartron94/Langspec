package sublime

type Binding struct {
	Scopes      []string            `json:"scopes,omitempty"`
	TokenScopes map[string][]string `json:"token_scopes,omitempty"`
}

type SublimeScopeManifest struct {
	BaseTokenScopes map[string]string  `json:"base_token_scopes,omitempty"`
	NodeBindings    map[string]Binding `json:"node_bindings,omitempty"`
	InvalidScope    string             `json:"invalid_scope,omitempty"`
}

type SublimeConfiguration struct {
	FileExtensions []string `json:"file_extensions,omitempty"`
	ScopeExtension string   `json:"scope_extension,omitempty"`

	Manifest SublimeScopeManifest `json:"scope_manifest"`
}
