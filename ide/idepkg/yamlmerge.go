// Unstable Build LLC ("COMPANY") CONFIDENTIAL
//
// Unpublished Copyright (c) 2017-2024 Unstable Build, All Rights Reserved.
//
// NOTICE: All information contained herein is, and remains the property of COMPANY.
// The intellectual and technical concepts contained herein are proprietary to
// COMPANY and may be covered by U.S. and Foreign Patents, patents in process,
// and are protected by trade secret or copyright law. Dissemination of this information
// or reproduction of this material is strictly forbidden unless prior written permission
// is obtained from COMPANY. Access to the source code contained herein is hereby
// forbidden to anyone except current COMPANY employees, managers or contractors who
// have executed Confidentiality and Non-disclosure agreements explicitly covering such access.
//
// The copyright notice above does not evidence any actual or intended publication or
// disclosure of this source code, which includes information that is confidential and/or
// proprietary, and is a trade secret, of COMPANY. ANY REPRODUCTION, MODIFICATION,
// DISTRIBUTION, PUBLIC  PERFORMANCE, OR PUBLIC DISPLAY OF OR THROUGH USE OF THIS SOURCE CODE
// WITHOUT  THE EXPRESS WRITTEN CONSENT OF COMPANY IS STRICTLY PROHIBITED, AND IN
// VIOLATION OF APPLICABLE LAWS AND INTERNATIONAL TREATIES. THE RECEIPT OR POSSESSION OF
// THIS SOURCE CODE AND/OR RELATED INFORMATION DOES NOT CONVEY OR IMPLY ANY RIGHTS TO
// REPRODUCE, DISCLOSE OR DISTRIBUTE ITS CONTENTS, OR TO MANUFACTURE, USE, OR SELL
// ANYTHING THAT IT MAY DESCRIBE, IN WHOLE OR IN PART.

package idepkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/syntax"
)

// loadOrCreateUserConfig reads a YAML file at path into a yaml.Node document.
// If the file does not exist or is empty, it logs a warning and returns an
// empty document containing an empty mapping node.
func loadOrCreateUserConfig(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read user config: %w", err)
	}
	if os.IsNotExist(err) || len(data) == 0 {
		if os.IsNotExist(err) {
			log.Warnf("user config %s does not exist, creating empty config", path)
		} else {
			log.Warnf("user config %s is empty, creating empty config", path)
		}
		return &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Tag: "!!map"},
			},
		}, nil
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal user config: %w", err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return &yaml.Node{
			Kind: yaml.DocumentNode,
			Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Tag: "!!map"},
			},
		}, nil
	}
	return &doc, nil
}

// mergeYAMLNodes additively merges src into dst. Both must be mapping
// nodes. For each key in src: if the key does not exist in dst, append
// it; if both values are mappings, recurse so genuinely-new sub-keys
// are added; otherwise leave dst's existing value untouched. The merge
// never overwrites a value the user already has set.
func mergeYAMLNodes(dst, src *yaml.Node) {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return
	}
	mergeMappings(dst, src)
}

func mergeMappings(dst, src *yaml.Node) {
	for i := 0; i < len(src.Content)-1; i += 2 {
		srcKey := src.Content[i]
		srcVal := src.Content[i+1]

		dstIdx := findMappingKey(dst, srcKey.Value)
		if dstIdx < 0 {
			dst.Content = append(dst.Content, cloneNode(srcKey), cloneNode(srcVal))
			continue
		}
		dstVal := dst.Content[dstIdx+1]
		if dstVal.Kind == yaml.MappingNode && srcVal.Kind == yaml.MappingNode {
			mergeMappings(dstVal, srcVal)
		}
	}
}

// applyConfigDiff merges the approved overlay diff src into dst. Both must
// be mapping nodes. Unlike mergeYAMLNodes, it overwrites an existing dst
// scalar with src's value. src carries only the keys the user agreed to
// apply (the prompt diff: genuinely new keys plus version-dependent keys
// whose resolved value changed, RUNE-225), so overwriting never clobbers
// an unrelated user customization.
func applyConfigDiff(dst, src *yaml.Node) {
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(src.Content)-1; i += 2 {
		srcKey := src.Content[i]
		srcVal := src.Content[i+1]

		dstIdx := findMappingKey(dst, srcKey.Value)
		if dstIdx < 0 {
			dst.Content = append(dst.Content, cloneNode(srcKey), cloneNode(srcVal))
			continue
		}
		dstVal := dst.Content[dstIdx+1]
		if dstVal.Kind == yaml.MappingNode && srcVal.Kind == yaml.MappingNode {
			applyConfigDiff(dstVal, srcVal)
			continue
		}
		dst.Content[dstIdx+1] = cloneNode(srcVal)
	}
}

// findMappingKey returns the index of the key node in a mapping's Content
// slice, or -1 if not found.
func findMappingKey(mapping *yaml.Node, key string) int {
	for i := 0; i < len(mapping.Content)-1; i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// configDiffMappingAtPath descends an applied config diff document following
// the given key path and returns the value node at that path, or nil if the
// path is absent. doc may be a DocumentNode (its first content child is used)
// or a MappingNode. Each intermediate node along the path must be a mapping;
// the final value may be of any kind.
func configDiffMappingAtPath(doc *yaml.Node, path ...string) *yaml.Node {
	if doc == nil || len(path) == 0 {
		return nil
	}
	node := doc
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil
		}
		node = node.Content[0]
	}
	for _, key := range path {
		if node.Kind != yaml.MappingNode {
			return nil
		}
		idx := findMappingKey(node, key)
		if idx < 0 {
			return nil
		}
		node = node.Content[idx+1]
	}
	return node
}

// configDiffTouchesPath reports whether an applied config diff includes the
// given nested key path.
func configDiffTouchesPath(doc *yaml.Node, path ...string) bool {
	return configDiffMappingAtPath(doc, path...) != nil
}

// addedExtensionIDs returns the keys added under the top-level "extensions"
// mapping of an applied config diff. It returns nil when the diff does not
// touch "extensions" or that key is not a (non-empty) mapping.
func addedExtensionIDs(doc *yaml.Node) []string {
	node := configDiffMappingAtPath(doc, "extensions")
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var ids []string
	for i := 0; i < len(node.Content)-1; i += 2 {
		ids = append(ids, node.Content[i].Value)
	}
	return ids
}

// expandRuneVars expands only the variables for which lookup returns
// ok; every other $VAR / ${VAR} reference is left verbatim in the
// result. It parses s as a single shell word with mvdan/sh so brace
// forms (${VAR}) are handled faithfully, and copies any source span it
// does not replace unchanged. If parsing fails (these are config
// templates, not arbitrary shell), s is returned unchanged.
func expandRuneVars(s string, lookup func(name string) (string, bool)) string {
	word, err := syntax.NewParser().Document(strings.NewReader(s))
	if err != nil || word == nil {
		return s
	}
	var b strings.Builder
	pos := 0
	syntax.Walk(word, func(node syntax.Node) bool {
		pe, ok := node.(*syntax.ParamExp)
		if !ok || pe.Param == nil {
			return true
		}
		val, ok := lookup(pe.Param.Value)
		if !ok {
			return true
		}
		start := int(pe.Pos().Offset())
		end := int(pe.End().Offset())
		if start < pos || end > len(s) {
			return true
		}
		b.WriteString(s[pos:start])
		b.WriteString(val)
		pos = end
		return true
	})
	b.WriteString(s[pos:])
	return b.String()
}

// expandNodeValues walks all scalar nodes in the tree and applies
// os.Expand with the given mapping function. Only string-tagged scalars
// are expanded (int, float, bool, null are skipped).
func expandNodeValues(n *yaml.Node, mapping func(string) string) {
	switch n.Kind {
	case yaml.DocumentNode, yaml.SequenceNode, yaml.MappingNode:
		for _, child := range n.Content {
			expandNodeValues(child, mapping)
		}
	case yaml.ScalarNode:
		switch n.Tag {
		case "!!int", "!!float", "!!bool", "!!null":
			return
		}
		n.Value = os.Expand(n.Value, mapping)
	}
}

// expandMapValues walks cfg in place, applying os.Expand to every string
// value using the given mapping function. Nested maps and slices are
// traversed recursively; non-string scalars are left untouched.
func expandMapValues(cfg map[string]any, mapping func(string) string) {
	for k, v := range cfg {
		cfg[k] = expandAnyValue(v, mapping)
	}
}

func expandAnyValue(v any, mapping func(string) string) any {
	switch t := v.(type) {
	case string:
		return os.Expand(t, mapping)
	case map[string]any:
		expandMapValues(t, mapping)
		return t
	case []any:
		for i, elem := range t {
			t[i] = expandAnyValue(elem, mapping)
		}
		return t
	default:
		return v
	}
}

// backupUserConfig copies path to path+".backup" before any write.
// It is a no-op if the source file does not exist.
func backupUserConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read for backup: %w", err)
	}
	backupPath := path + ".backup"
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return "", fmt.Errorf("write backup: %w", err)
	}
	return backupPath, nil
}

// writeYAMLAtomic writes doc to path atomically. It creates a temp file in
// the same directory, encodes the document, verifies that the written content
// contains all expected keys from expected, then renames.
func writeYAMLAtomic(path string, doc *yaml.Node, expected *yaml.Node) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}

	// Verify round-trip
	readBack, err := os.ReadFile(tmpName)
	if err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("read back: %w", err)
	}
	var written yaml.Node
	if err := yaml.Unmarshal(readBack, &written); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("unmarshal read back: %w", err)
	}
	if written.Kind != yaml.DocumentNode || len(written.Content) == 0 {
		_ = os.Remove(tmpName)
		return fmt.Errorf("written file has unexpected structure")
	}
	if err := verifyMerge(written.Content[0], expected); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("verification failed: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// verifyMerge recursively walks expected mapping keys and confirms each
// is present in written. Scalar values are not compared because the
// additive merge intentionally preserves any value the user already
// has set even when the package overlay declares a different value.
func verifyMerge(written, expected *yaml.Node) error {
	if expected.Kind != yaml.MappingNode {
		return nil
	}
	if written.Kind != yaml.MappingNode {
		return fmt.Errorf("expected mapping node, got kind %d", written.Kind)
	}
	for i := 0; i < len(expected.Content)-1; i += 2 {
		key := expected.Content[i].Value
		expVal := expected.Content[i+1]

		wIdx := findMappingKey(written, key)
		if wIdx < 0 {
			return fmt.Errorf("key %q missing from written config", key)
		}
		wVal := written.Content[wIdx+1]

		if expVal.Kind == yaml.MappingNode && wVal.Kind == yaml.MappingNode {
			if err := verifyMerge(wVal, expVal); err != nil {
				return fmt.Errorf("key %q: %w", key, err)
			}
		}
	}
	return nil
}

// cloneNode returns a deep copy of a yaml.Node tree.
func cloneNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	clone := &yaml.Node{
		Kind:        n.Kind,
		Style:       n.Style,
		Tag:         n.Tag,
		Value:       n.Value,
		Anchor:      n.Anchor,
		Alias:       cloneNode(n.Alias),
		HeadComment: n.HeadComment,
		LineComment: n.LineComment,
		FootComment: n.FootComment,
		Line:        n.Line,
		Column:      n.Column,
	}
	if len(n.Content) > 0 {
		clone.Content = make([]*yaml.Node, len(n.Content))
		for i, child := range n.Content {
			clone.Content[i] = cloneNode(child)
		}
	}
	return clone
}
