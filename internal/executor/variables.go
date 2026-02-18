package executor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// varPattern matches ${function_name.field.subfield} references.
// Supports dotted paths of arbitrary depth, e.g. ${grpc.latency_ms} or ${tcp.nested.value}.
var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// VariableResolver stores JSON outputs from already-executed functions and resolves
// ${function_name.field_path} references in subsequent function parameters.
//
// Usage:
//
//	vr := NewVariableResolver()
//	vr.AddResult("check_tcp_health", `{"port":50051,"interface":"eth0"}`)
//	resolved, err := vr.ResolveParams(params)
type VariableResolver struct {
	// results maps function name -> parsed JSON (map or scalar).
	results map[string]interface{}
}

// NewVariableResolver creates an empty resolver.
func NewVariableResolver() *VariableResolver {
	return &VariableResolver{
		results: make(map[string]interface{}),
	}
}

// AddResult stores the JSON output of a completed function execution.
// The output is parsed eagerly so resolution is fast.
// Non-JSON output (plain strings) is stored as a raw string under the key "value".
func (vr *VariableResolver) AddResult(functionName string, jsonOutput string) {
	if jsonOutput == "" {
		return
	}

	var parsed interface{}
	if err := json.Unmarshal([]byte(jsonOutput), &parsed); err != nil {
		// Not JSON — store as a plain string so ${func.value} still works.
		vr.results[functionName] = map[string]interface{}{
			"value":  jsonOutput,
			"output": jsonOutput,
		}
		return
	}

	vr.results[functionName] = parsed
}

// HasResult reports whether a result exists for the given function name.
func (vr *VariableResolver) HasResult(functionName string) bool {
	_, ok := vr.results[functionName]
	return ok
}

// ResolveParams walks a params map and resolves all ${...} placeholders in string
// values. Non-string values are passed through unchanged.
// Returns a new map; the original is not modified.
func (vr *VariableResolver) ResolveParams(params map[string]interface{}) (map[string]interface{}, error) {
	if params == nil {
		return nil, nil
	}

	resolved := make(map[string]interface{}, len(params))
	for key, val := range params {
		resolvedVal, err := vr.resolveValue(val)
		if err != nil {
			return nil, fmt.Errorf("param %q: %w", key, err)
		}
		resolved[key] = resolvedVal
	}
	return resolved, nil
}

// Resolve resolves all ${...} placeholders in a single string.
// If the entire string is a single placeholder and the resolved value is a
// non-string type, the native type is returned as a string representation
// suitable for further coercion by the executor.
// Returns the resolved string, or an error if a reference cannot be found.
func (vr *VariableResolver) Resolve(value string) (string, error) {
	// Fast path: no placeholder.
	if !strings.Contains(value, "${") {
		return value, nil
	}

	var resolveErr error
	result := varPattern.ReplaceAllStringFunc(value, func(match string) string {
		if resolveErr != nil {
			return match
		}
		// Extract the reference path from ${...}
		ref := match[2 : len(match)-1] // strip ${ and }
		resolved, err := vr.resolveReference(ref)
		if err != nil {
			resolveErr = err
			return match
		}
		return fmt.Sprintf("%v", resolved)
	})

	if resolveErr != nil {
		return "", resolveErr
	}
	return result, nil
}

// ============================================================================
// Internal helpers
// ============================================================================

// resolveValue resolves placeholders in a single parameter value.
// Handles string, map, and slice values recursively.
func (vr *VariableResolver) resolveValue(val interface{}) (interface{}, error) {
	switch v := val.(type) {
	case string:
		// Check if the entire value is a single placeholder — if so, return the
		// native resolved type (e.g. int) rather than a stringified version.
		if native, ok := vr.tryResolveNative(v); ok {
			return native, nil
		}
		// Otherwise do string interpolation.
		return vr.Resolve(v)

	case map[string]interface{}:
		// Recurse into nested maps.
		return vr.ResolveParams(v)

	case []interface{}:
		// Recurse into slices.
		out := make([]interface{}, len(v))
		for i, item := range v {
			resolved, err := vr.resolveValue(item)
			if err != nil {
				return nil, fmt.Errorf("[%d]: %w", i, err)
			}
			out[i] = resolved
		}
		return out, nil

	default:
		// Numbers, bools, nil — pass through unchanged.
		return val, nil
	}
}

// tryResolveNative checks whether value is exactly a single ${...} placeholder.
// If it is, it returns the resolved native value (preserving int/float/bool types)
// and true. Otherwise returns nil, false.
func (vr *VariableResolver) tryResolveNative(value string) (interface{}, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "${") || !strings.HasSuffix(trimmed, "}") {
		return nil, false
	}
	// Check there's only one placeholder and it covers the whole string.
	matches := varPattern.FindAllString(trimmed, -1)
	if len(matches) != 1 || matches[0] != trimmed {
		return nil, false
	}

	ref := trimmed[2 : len(trimmed)-1]
	resolved, err := vr.resolveReference(ref)
	if err != nil {
		return nil, false
	}
	return resolved, true
}

// resolveReference resolves a dotted path like "function_name.field.subfield"
// against the stored results.
func (vr *VariableResolver) resolveReference(ref string) (interface{}, error) {
	parts := strings.SplitN(ref, ".", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("empty reference %q", ref)
	}

	funcName := parts[0]
	result, ok := vr.results[funcName]
	if !ok {
		return nil, fmt.Errorf("no result available for function %q (reference: ${%s})", funcName, ref)
	}

	// If no field path, return the whole result.
	if len(parts) == 1 || parts[1] == "" {
		return result, nil
	}

	// Walk the dotted path.
	fieldPath := parts[1]
	return walkPath(result, fieldPath, ref)
}

// walkPath traverses a dotted field path on a parsed JSON value.
// Supports map keys and array indices (e.g. "items.0.name").
func walkPath(current interface{}, path string, originalRef string) (interface{}, error) {
	segments := strings.Split(path, ".")

	for _, seg := range segments {
		if seg == "" {
			return nil, fmt.Errorf("empty path segment in ${%s}", originalRef)
		}

		switch node := current.(type) {
		case map[string]interface{}:
			val, ok := node[seg]
			if !ok {
				// Collect available keys for a helpful error.
				keys := make([]string, 0, len(node))
				for k := range node {
					keys = append(keys, k)
				}
				return nil, fmt.Errorf(
					"field %q not found in ${%s}; available fields: [%s]",
					seg, originalRef, strings.Join(keys, ", "),
				)
			}
			current = val

		case []interface{}:
			idx, err := strconv.Atoi(seg)
			if err != nil {
				return nil, fmt.Errorf(
					"cannot index array with non-integer %q in ${%s}", seg, originalRef,
				)
			}
			if idx < 0 || idx >= len(node) {
				return nil, fmt.Errorf(
					"array index %d out of range (len=%d) in ${%s}", idx, len(node), originalRef,
				)
			}
			current = node[idx]

		default:
			return nil, fmt.Errorf(
				"cannot traverse into %T at segment %q in ${%s}", current, seg, originalRef,
			)
		}
	}

	return current, nil
}

// ContainsVariables reports whether a params map has any ${...} placeholders.
// Useful for skipping resolution overhead when not needed.
func ContainsVariables(params map[string]interface{}) bool {
	for _, v := range params {
		if containsVar(v) {
			return true
		}
	}
	return false
}

func containsVar(v interface{}) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, "${")
	case map[string]interface{}:
		return ContainsVariables(t)
	case []interface{}:
		for _, item := range t {
			if containsVar(item) {
				return true
			}
		}
	}
	return false
}
