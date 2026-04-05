package toolchain

import (
	"fmt"
	"foundation/system"
	"langspec/dsl"
)

func ExtractOutputPathsFromPragma(pragma dsl.ToolPragma, pathKey string) ([]string, error) {
	outputPathValue := pragma.Settings[pathKey]
	var rawPaths []string

	if path, cnvOk := outputPathValue.(string); cnvOk {
		rawPaths = append(rawPaths, path)
	} else if pathArray, cnvOk := outputPathValue.([]string); cnvOk {
		rawPaths = pathArray
	} else {
		return nil, fmt.Errorf("engine-error: output paths neither single value nor array")
	}

	var resolvedPaths []string
	for _, rp := range rawPaths {
		resolved, err := system.PathResolveWorkspace(rp)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve path %s: %w", rp, err)
		}
		resolvedPaths = append(resolvedPaths, resolved)
	}

	return resolvedPaths, nil
}
