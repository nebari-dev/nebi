// Package diff provides semantic diff engines for pixi.toml and pixi.lock files.
//
// The TOML diff engine parses pixi.toml files structurally and produces
// semantic diffs (e.g., "numpy upgraded from 2.0 to 2.4") rather than
// raw line-by-line text diffs.
package diff

import (
	"fmt"
	"sort"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

// ChangeType represents the type of change in a diff.
type ChangeType string

const (
	ChangeAdded    ChangeType = "added"
	ChangeRemoved  ChangeType = "removed"
	ChangeModified ChangeType = "modified"
)

// Change represents a single semantic change in a TOML file.
type Change struct {
	Section  string     `json:"section"`
	Key      string     `json:"key"`
	Type     ChangeType `json:"type"`
	OldValue string     `json:"old_value,omitempty"`
	NewValue string     `json:"new_value,omitempty"`
}

// TomlDiff represents the semantic difference between two TOML files.
type TomlDiff struct {
	Changes []Change `json:"changes"`
}

// HasChanges returns true if there are any differences.
func (d *TomlDiff) HasChanges() bool {
	return len(d.Changes) > 0
}

// Added returns all added changes.
func (d *TomlDiff) Added() []Change {
	return d.filterByType(ChangeAdded)
}

// Removed returns all removed changes.
func (d *TomlDiff) Removed() []Change {
	return d.filterByType(ChangeRemoved)
}

// Modified returns all modified changes.
func (d *TomlDiff) Modified() []Change {
	return d.filterByType(ChangeModified)
}

func (d *TomlDiff) filterByType(t ChangeType) []Change {
	var result []Change
	for _, c := range d.Changes {
		if c.Type == t {
			result = append(result, c)
		}
	}
	return result
}

// CompareToml parses two TOML contents and produces a semantic diff.
func CompareToml(oldContent, newContent []byte) (*TomlDiff, error) {
	var oldMap, newMap map[string]interface{}

	if err := toml.Unmarshal(oldContent, &oldMap); err != nil {
		return nil, fmt.Errorf("failed to parse old TOML: %w", err)
	}
	if err := toml.Unmarshal(newContent, &newMap); err != nil {
		return nil, fmt.Errorf("failed to parse new TOML: %w", err)
	}

	diff := &TomlDiff{}
	compareMaps(oldMap, newMap, "", diff)
	return diff, nil
}

// compareMaps recursively compares two maps and appends changes to the diff.
func compareMaps(oldMap, newMap map[string]interface{}, prefix string, diff *TomlDiff) {
	// Collect all keys
	allKeys := make(map[string]bool)
	for k := range oldMap {
		allKeys[k] = true
	}
	for k := range newMap {
		allKeys[k] = true
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		oldVal, oldExists := oldMap[key]
		newVal, newExists := newMap[key]

		section := prefix
		if section == "" {
			section = key
		}
		fullKey := key
		if prefix != "" {
			fullKey = key
		}

		if !oldExists {
			// Key was added
			addChangesForValue(section, fullKey, newVal, ChangeAdded, diff)
			continue
		}

		if !newExists {
			// Key was removed
			addChangesForValue(section, fullKey, oldVal, ChangeRemoved, diff)
			continue
		}

		// Both exist - compare values
		oldSubMap, oldIsMap := oldVal.(map[string]interface{})
		newSubMap, newIsMap := newVal.(map[string]interface{})

		if oldIsMap && newIsMap {
			// Both are maps - recurse with section context
			subPrefix := key
			if prefix != "" {
				subPrefix = prefix + "." + key
			}
			compareMaps(oldSubMap, newSubMap, subPrefix, diff)
		} else {
			// Compare as values
			oldStr := formatValue(oldVal)
			newStr := formatValue(newVal)
			if oldStr != newStr {
				diff.Changes = append(diff.Changes, Change{
					Section:  section,
					Key:      fullKey,
					Type:     ChangeModified,
					OldValue: oldStr,
					NewValue: newStr,
				})
			}
		}
	}
}

// addChangesForValue adds changes for a value (handling nested maps).
func addChangesForValue(section, key string, val interface{}, changeType ChangeType, diff *TomlDiff) {
	subMap, isMap := val.(map[string]interface{})
	if isMap {
		keys := sortedKeys(subMap)
		for _, k := range keys {
			change := Change{
				Section: section,
				Key:     k,
				Type:    changeType,
			}
			if changeType == ChangeAdded {
				change.NewValue = formatValue(subMap[k])
			} else {
				change.OldValue = formatValue(subMap[k])
			}
			diff.Changes = append(diff.Changes, change)
		}
	} else {
		change := Change{
			Section: section,
			Key:     key,
			Type:    changeType,
		}
		if changeType == ChangeAdded {
			change.NewValue = formatValue(val)
		} else {
			change.OldValue = formatValue(val)
		}
		diff.Changes = append(diff.Changes, change)
	}
}

// formatValue converts a TOML value to a string representation.
func formatValue(val interface{}) string {
	if val == nil {
		return ""
	}

	switch v := val.(type) {
	case string:
		return v
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		return fmt.Sprintf("%t", v)
	case []interface{}:
		parts := make([]string, len(v))
		for i, item := range v {
			parts[i] = formatValue(item)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case map[string]interface{}:
		return fmt.Sprintf("%v", v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// sortedKeys returns sorted keys of a map.
func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// FormatUnifiedDiff formats a TomlDiff as a unified diff string.
func FormatUnifiedDiff(diff *TomlDiff, sourceLabel, targetLabel string) string {
	if !diff.HasChanges() {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("--- %s\n", sourceLabel))
	sb.WriteString(fmt.Sprintf("+++ %s\n", targetLabel))
	sb.WriteString("@@ pixi.toml @@\n")

	// Group changes by section
	sectionChanges := make(map[string][]Change)
	var sectionOrder []string
	for _, c := range diff.Changes {
		if _, exists := sectionChanges[c.Section]; !exists {
			sectionOrder = append(sectionOrder, c.Section)
		}
		sectionChanges[c.Section] = append(sectionChanges[c.Section], c)
	}

	for _, section := range sectionOrder {
		sb.WriteString(fmt.Sprintf(" [%s]\n", section))
		for _, c := range sectionChanges[section] {
			switch c.Type {
			case ChangeAdded:
				sb.WriteString(fmt.Sprintf("+%s = %q\n", c.Key, c.NewValue))
			case ChangeRemoved:
				sb.WriteString(fmt.Sprintf("-%s = %q\n", c.Key, c.OldValue))
			case ChangeModified:
				sb.WriteString(fmt.Sprintf("-%s = %q\n", c.Key, c.OldValue))
				sb.WriteString(fmt.Sprintf("+%s = %q\n", c.Key, c.NewValue))
			}
		}
	}

	return sb.String()
}
