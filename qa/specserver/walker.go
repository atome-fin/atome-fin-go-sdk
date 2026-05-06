package specserver

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// walk visits every operation in the loaded spec and populates
// s.Operations with each op's RequiredBody / RequiredQuery sets.
//
// The walker is a small hand-rolled OpenAPI-3 schema descender. It
// deliberately does NOT model the full OpenAPI dialect — it knows
// just enough to resolve $refs into the components.requestBodies /
// components.schemas tables, recurse through nested object /
// $ref-array property trees, and accumulate the union of every
// `required:` array it encounters as a flat list of dotted /
// bracketed field paths.
//
// Field-path encoding:
//   - "fieldName" — top-level required body field
//   - "outer.inner" — required field on a nested object property
//   - "items[].field" — required field on each element of an array
//     of objects
//
// The dispatcher walks the decoded request body using the same
// encoding so paths in error messages match what the developer sees
// in the spec.
func (s *Spec) walk() error {
	for path, pathNode := range s.raw.Paths {
		// pathNode is a YAML mapping node: method → operation.
		if pathNode.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i < len(pathNode.Content); i += 2 {
			methodNode := pathNode.Content[i]
			opNode := pathNode.Content[i+1]
			method := strings.ToUpper(methodNode.Value)
			switch method {
			case "GET", "POST", "PUT", "DELETE", "PATCH":
				op, err := s.parseOperation(method, path, opNode)
				if err != nil {
					return fmt.Errorf("%s %s: %w", method, path, err)
				}
				s.Operations[opKey(method, path)] = op
			default:
				// Skip non-method keys (parameters, summary, etc.).
			}
		}
	}
	return nil
}

// parseOperation extracts RequiredBody / RequiredQuery for one
// (method, path) pair.
func (s *Spec) parseOperation(method, path string, opNode *yaml.Node) (Operation, error) {
	op := Operation{Method: method, Path: path}

	// --- parameters: collect required query + header names ---
	if params := mappingChild(opNode, "parameters"); params != nil {
		for _, p := range params.Content {
			loc := scalarChild(p, "in")
			req := scalarChild(p, "required")
			name := scalarChild(p, "name")
			if req != "true" || name == "" {
				continue
			}
			switch loc {
			case "query":
				op.RequiredQuery = append(op.RequiredQuery, name)
			case "header":
				// Authorization is on every signed endpoint; out of
				// scope per architect §1.7 (signature validation is
				// the sign package's job, not the spec server's).
				if name == "Authorization" {
					continue
				}
				op.RequiredHeader = append(op.RequiredHeader, name)
			}
		}
	}

	// --- request body (POST-relevant) ---
	if body := mappingChild(opNode, "requestBody"); body != nil {
		schema, err := s.bodySchema(body)
		if err != nil {
			return op, fmt.Errorf("requestBody: %w", err)
		}
		if schema != nil {
			seen := map[string]bool{}
			collectRequired(schema, "", &op.RequiredBody, seen, s, 0)
		}
	}

	stableSort(op.RequiredBody)
	stableSort(op.RequiredQuery)
	stableSort(op.RequiredHeader)
	return op, nil
}

// bodySchema resolves the requestBody node down to its inner schema
// node, following the one $ref hop the spec uses
// (`#/components/requestBodies/Foo` → that requestBody's inline
// schema). Returns nil schema if requestBody is absent or non-JSON.
func (s *Spec) bodySchema(body *yaml.Node) (*yaml.Node, error) {
	// Either inline `content: application/json: schema:` or a
	// `$ref: '#/components/requestBodies/Foo'`.
	if ref := scalarChild(body, "$ref"); ref != "" {
		name := strings.TrimPrefix(ref, "#/components/requestBodies/")
		rb, ok := s.raw.Components.RequestBodies[name]
		if !ok {
			return nil, fmt.Errorf("$ref %q: requestBody not found in components", ref)
		}
		body = &rb
	}
	content := mappingChild(body, "content")
	if content == nil {
		return nil, nil
	}
	appJSON := mappingChild(content, "application/json")
	if appJSON == nil {
		return nil, nil
	}
	return mappingChild(appJSON, "schema"), nil
}

// collectRequired walks a schema subtree and appends every required
// field path to *out. Uses seen[path] to dedupe across $ref reuse.
//
// Walk algorithm:
//  1. Resolve $ref (if present) to the referenced schema.
//  2. If the schema has a `required:` array, every name in it
//     becomes a required path at the current prefix.
//  3. For each property whose value is itself a schema, recurse.
//  4. For array properties (`type: array, items: ...`), recurse into
//     items with a "[]" suffix on the prefix.
//
// Depth is bounded (depth <= maxDepth) as a runaway-recursion guard
// against pathological self-referential schemas.
func collectRequired(node *yaml.Node, prefix string, out *[]string, seen map[string]bool, s *Spec, depth int) {
	const maxDepth = 12
	if node == nil || depth > maxDepth {
		return
	}
	// Resolve $ref before anything else.
	if ref := scalarChild(node, "$ref"); ref != "" {
		resolved := s.resolveRef(ref)
		if resolved == nil {
			return
		}
		node = resolved
	}

	// Required array at this level.
	if req := mappingChild(node, "required"); req != nil && req.Kind == yaml.SequenceNode {
		for _, n := range req.Content {
			path := joinPath(prefix, n.Value)
			if !seen[path] {
				seen[path] = true
				*out = append(*out, path)
			}
		}
	}

	// Properties: recurse into each.
	if props := mappingChild(node, "properties"); props != nil && props.Kind == yaml.MappingNode {
		for i := 0; i < len(props.Content); i += 2 {
			name := props.Content[i].Value
			child := props.Content[i+1]
			collectRequired(child, joinPath(prefix, name), out, seen, s, depth+1)
		}
	}

	// Array items: recurse with "[]" suffix.
	if scalarChild(node, "type") == "array" {
		if items := mappingChild(node, "items"); items != nil {
			collectRequired(items, prefix+"[]", out, seen, s, depth+1)
		}
	}
}

// resolveRef follows a `#/components/{requestBodies,schemas}/Name`
// ref and returns the referenced node. Other ref shapes return nil.
func (s *Spec) resolveRef(ref string) *yaml.Node {
	const schemaPrefix = "#/components/schemas/"
	const reqBodyPrefix = "#/components/requestBodies/"
	switch {
	case strings.HasPrefix(ref, schemaPrefix):
		name := strings.TrimPrefix(ref, schemaPrefix)
		if n, ok := s.raw.Components.Schemas[name]; ok {
			return &n
		}
	case strings.HasPrefix(ref, reqBodyPrefix):
		name := strings.TrimPrefix(ref, reqBodyPrefix)
		if n, ok := s.raw.Components.RequestBodies[name]; ok {
			return &n
		}
	}
	return nil
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	if strings.HasSuffix(prefix, "[]") {
		return prefix + "." + name
	}
	return prefix + "." + name
}

// mappingChild returns the value node for `key` in a mapping node,
// or nil if absent / non-mapping.
func mappingChild(node *yaml.Node, key string) *yaml.Node {
	if node == nil {
		return nil
	}
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// scalarChild returns the .Value of a scalar key in a mapping, or ""
// if absent / non-scalar.
func scalarChild(node *yaml.Node, key string) string {
	v := mappingChild(node, key)
	if v == nil || v.Kind != yaml.ScalarNode {
		return ""
	}
	return v.Value
}
