package cliutil

import (
	"errors"
	"fmt"
	"langspec/dsl"
	"langspec/toolchain"
	"reflect"
)

func keyStringFromReflectValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}
	return fmt.Sprint(v.Interface())
}

func stringSliceFromReflectValue(v reflect.Value) []string {
	if !v.IsValid() || v.Kind() != reflect.Slice {
		return nil
	}
	out := make([]string, 0, v.Len())
	for i := 0; i < v.Len(); i++ {
		out = append(out, fmt.Sprint(v.Index(i).Interface()))
	}
	return out
}

func semanticManifestFromReflectValue(manifestAny any) (toolchain.SemanticManifest[string, string], error) {
	out := toolchain.SemanticManifest[string, string]{
		BaseTokenScopes: map[string]string{},
		NodeBindings:    map[string]toolchain.NodeBinding[string]{},
	}

	mv := reflect.ValueOf(manifestAny)
	if !mv.IsValid() {
		return out, errors.New("invalid Manifest field")
	}
	if mv.Kind() == reflect.Pointer {
		if mv.IsNil() {
			return out, errors.New("manifest is nil")
		}
		mv = mv.Elem()
	}
	if mv.Kind() != reflect.Struct {
		return out, fmt.Errorf("manifest field must be struct, got %s", mv.Kind())
	}

	invalidScope := mv.FieldByName("InvalidScope")
	if invalidScope.IsValid() {
		out.InvalidScope = fmt.Sprint(invalidScope.Interface())
	}

	baseTokenScopes := mv.FieldByName("BaseTokenScopes")
	if baseTokenScopes.IsValid() && baseTokenScopes.Kind() == reflect.Map {
		iter := baseTokenScopes.MapRange()
		for iter.Next() {
			out.BaseTokenScopes[keyStringFromReflectValue(iter.Key())] = fmt.Sprint(iter.Value().Interface())
		}
	}

	nodeBindings := mv.FieldByName("NodeBindings")
	if nodeBindings.IsValid() && nodeBindings.Kind() == reflect.Map {
		iter := nodeBindings.MapRange()
		for iter.Next() {
			bindingVal := iter.Value()
			if bindingVal.Kind() == reflect.Pointer {
				if bindingVal.IsNil() {
					continue
				}
				bindingVal = bindingVal.Elem()
			}
			b := toolchain.NodeBinding[string]{
				TokenScopes: map[string][]string{},
			}
			if bindingVal.Kind() == reflect.Struct {
				scopesField := bindingVal.FieldByName("Scopes")
				b.Scopes = stringSliceFromReflectValue(scopesField)
				metaField := bindingVal.FieldByName("MetaScope")
				if metaField.IsValid() {
					b.MetaScope = fmt.Sprint(metaField.Interface())
				}
				tokenScopesField := bindingVal.FieldByName("TokenScopes")
				if tokenScopesField.IsValid() && tokenScopesField.Kind() == reflect.Map {
					tokenIter := tokenScopesField.MapRange()
					for tokenIter.Next() {
						b.TokenScopes[keyStringFromReflectValue(tokenIter.Key())] = stringSliceFromReflectValue(tokenIter.Value())
					}
				}
			}
			out.NodeBindings[keyStringFromReflectValue(iter.Key())] = b
		}
	}

	return out, nil
}

/*
RunInMemorySublimeToolchainsFromAny runs the same pipeline as RunInMemorySublimeToolchains using
reflection over a config value shaped like InMemorySublimeRunnerConfig (fields SpecPath,
Manifest, OverrideFactory, FileExtensions, ScopeExtension). Intended for tiny generated mains
from shell wrappers; prefer the typed API when possible.
*/
func RunInMemorySublimeToolchainsFromAny(cfgAny any) error {
	cv := reflect.ValueOf(cfgAny)
	if cv.Kind() == reflect.Pointer {
		if cv.IsNil() {
			return errors.New("config is nil")
		}
		cv = cv.Elem()
	}
	if cv.Kind() != reflect.Struct {
		return fmt.Errorf("config must be struct, got %s", cv.Kind())
	}

	specPathField := cv.FieldByName("SpecPath")
	manifestField := cv.FieldByName("Manifest")
	fileExtensionsField := cv.FieldByName("FileExtensions")
	scopeExtensionField := cv.FieldByName("ScopeExtension")
	overrideFactoryField := cv.FieldByName("OverrideFactory")

	if !specPathField.IsValid() || !manifestField.IsValid() {
		return errors.New("config missing required fields (SpecPath, Manifest)")
	}

	specPath := fmt.Sprint(specPathField.Interface())
	stringManifest, err := semanticManifestFromReflectValue(manifestField.Interface())
	if err != nil {
		return err
	}

	fileExtensions := stringSliceFromReflectValue(fileExtensionsField)
	scopeExtension := fmt.Sprint(scopeExtensionField.Interface())

	var overrideFactory toolchain.SublimeInMemoryOverrideFactory[dsl.LangSpecParserNodeKind]
	if overrideFactoryField.IsValid() && !overrideFactoryField.IsNil() {
		typed, ok := overrideFactoryField.Interface().(toolchain.SublimeInMemoryOverrideFactory[dsl.LangSpecParserNodeKind])
		if !ok {
			return errors.New("OverrideFactory has unsupported function signature")
		}
		overrideFactory = typed
	}

	return runInMemorySublimeToolchainsFromStringConfig(
		specPath,
		stringManifest,
		overrideFactory,
		fileExtensions,
		scopeExtension,
	)
}
