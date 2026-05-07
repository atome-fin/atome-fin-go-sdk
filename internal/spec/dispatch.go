package spec

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ValidateBody walks a decoded JSON body against the supplied
// required-field paths and returns the first missing field path,
// or "" when every required field is present.
//
// Path syntax:
//   - "field"           — top-level field
//   - "outer.inner"     — nested object property
//   - "items[].field"   — required field on each element of an
//     array of objects (every element is checked; first miss
//     reported)
//
// "Present" is defined as: the JSON document has a key at the
// path AND the key's value is not JSON null. Empty strings, zero
// numbers, empty arrays, and empty objects are all considered
// present — presence detection only, never constraint validation.
// Type / enum / maxLength checks are §1.7 out-of-scope per
// SPEC_ASSERTION_TEST_DESIGN.md.
//
// On JSON-decode failure returns ("", err); on a missing field
// returns (path, nil); on success returns ("", nil).
func ValidateBody(body []byte, required []string) (string, error) {
	return validateBody(body, required)
}

func validateBody(body []byte, required []string) (string, error) {
	if len(required) == 0 {
		return "", nil
	}
	// Loose decode — the framework asserts presence, not types.
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return "", fmt.Errorf("invalid JSON body: %w", err)
	}
	for _, path := range required {
		if missing := findMissing(doc, path); missing != "" {
			return missing, nil
		}
	}
	return "", nil
}

// findMissing returns the rendered path of the first missing
// required field at or under `path`, or "" if everything required
// at this path is present.
func findMissing(doc any, path string) string {
	segs := splitPath(path)
	return descend(doc, segs, 0, "")
}

// descend walks one level at a time. `rendered` is the user-facing
// path-so-far (with concrete array index substituted in for "[]").
func descend(node any, segs []string, i int, rendered string) string {
	if i >= len(segs) {
		// Reached the leaf level — node must be present (non-null).
		if node == nil {
			return rendered
		}
		return ""
	}
	seg := segs[i]
	if strings.HasSuffix(seg, "[]") {
		propName := strings.TrimSuffix(seg, "[]")
		next := childOf(node, propName)
		if next == nil {
			// Array property itself absent — this means there's
			// nothing to check inside. The required-set entry was
			// `outer[].inner`, but if `outer` is optional and not
			// sent, sub-fields don't apply.
			return ""
		}
		arr, ok := next.([]any)
		if !ok {
			return ""
		}
		// Empty array: nothing to validate.
		for idx, elem := range arr {
			r := joinRendered(joinRendered(rendered, propName), fmt.Sprintf("[%d]", idx))
			if missing := descend(elem, segs, i+1, r); missing != "" {
				return missing
			}
		}
		return ""
	}
	next := childOf(node, seg)
	r := joinRendered(rendered, seg)
	if next == nil {
		return r
	}
	if i == len(segs)-1 {
		// Leaf — present.
		return ""
	}
	return descend(next, segs, i+1, r)
}

func childOf(node any, key string) any {
	m, ok := node.(map[string]any)
	if !ok {
		return nil
	}
	v, present := m[key]
	if !present {
		// JSON null is present-but-null; treat as missing for the
		// presence check (matches the spec's "required: true"
		// semantics).
		return nil
	}
	return v
}

// splitPath turns "outer[].inner.deep" into ["outer[]", "inner", "deep"].
func splitPath(p string) []string {
	if p == "" {
		return nil
	}
	parts := strings.Split(p, ".")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, part)
	}
	return out
}

func joinRendered(prefix, seg string) string {
	if prefix == "" {
		return seg
	}
	// Index segments come in as "[N]" — concatenate without dot.
	if strings.HasPrefix(seg, "[") {
		return prefix + seg
	}
	return prefix + "." + seg
}
