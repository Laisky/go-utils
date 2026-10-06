package utils

import (
	"github.com/Laisky/errors/v2"
	"github.com/Laisky/zap"

	"github.com/Laisky/go-utils/v6/log"
)

// FlattenMapSafe returns a detached flat map, rejecting ambiguous paths, an empty
// delimiter, or nesting deeper than 128 levels. It never modifies input maps.
// Empty nested maps remain values. Non-map values are copied shallowly.
func FlattenMapSafe(data map[string]any, delimiter string) (map[string]any, error) {
	return flattenMapChecked(data, delimiter, true)
}

// FlattenMap flattens data in place only when its complete input is unambiguous.
// Invalid input is left unchanged and produces one bounded warning. Empty nested
// maps are omitted for compatibility with the original function.
//
// Deprecated: use FlattenMapSafe and handle its error before trusting normalized data.
func FlattenMap(data map[string]any, delimiter string) {
	flat, err := flattenMapChecked(data, delimiter, false)
	if err != nil {
		log.Shared.Warn("flatten map rejected; use FlattenMapSafe to handle errors", zap.Error(err))
		return
	}
	clear(data)
	for key, value := range flat {
		data[key] = value
	}
}

// flattenMapChecked builds output separately and reserves every leaf path before
// publication. KeepEmpty controls only emission, never collision detection.
func flattenMapChecked(data map[string]any, delimiter string, keepEmpty bool) (map[string]any, error) {
	if delimiter == "" {
		return nil, errors.New("flatten map: delimiter must not be empty")
	}
	if data == nil {
		return nil, nil
	}
	out := make(map[string]any)
	seen := make(map[string]struct{})
	var visit func(map[string]any, string, bool, int) error
	visit = func(input map[string]any, prefix string, hasPrefix bool, depth int) error {
		if depth > 128 {
			return errors.New("flatten map: maximum nesting depth exceeded")
		}
		for key, value := range input {
			name := key
			if hasPrefix {
				name = prefix + delimiter + key
			}
			nested, isMap := value.(map[string]any)
			if isMap && len(nested) != 0 {
				if err := visit(nested, name, true, depth+1); err != nil {
					return err
				}
				continue
			}
			if _, exists := seen[name]; exists {
				return errors.New("flatten map: ambiguous flattened path")
			}
			seen[name] = struct{}{}
			if isMap {
				if keepEmpty {
					if nested == nil {
						out[name] = (map[string]any)(nil)
					} else {
						out[name] = map[string]any{}
					}
				}
				continue
			}
			out[name] = value
		}
		return nil
	}
	if err := visit(data, "", false, 0); err != nil {
		return nil, errors.WithStack(err)
	}
	return out, nil
}
