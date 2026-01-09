// Command filter-openapi filters an OpenAPI spec to only include
// endpoints with the x-cli extension set to true
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"
)

func main() {
	inputFile := flag.String("input", "docs/swagger.json", "Input OpenAPI spec file")
	outputFile := flag.String("output", "", "Output file (default: stdout)")
	flag.Parse()

	// Read input file
	data, err := os.ReadFile(*inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input file: %v\n", err)
		os.Exit(1)
	}

	// Parse JSON
	var spec map[string]interface{}
	if err := json.Unmarshal(data, &spec); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing JSON: %v\n", err)
		os.Exit(1)
	}

	// Filter paths
	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: no paths found in spec\n")
		os.Exit(1)
	}

	filteredPaths := make(map[string]interface{})
	usedTags := make(map[string]bool)

	for path, methods := range paths {
		methodsMap, ok := methods.(map[string]interface{})
		if !ok {
			continue
		}

		filteredMethods := make(map[string]interface{})
		for method, operation := range methodsMap {
			opMap, ok := operation.(map[string]interface{})
			if !ok {
				continue
			}

			// Check if operation has x-cli extension set to true
			hasCli := false
			if xCli, ok := opMap["x-cli"]; ok {
				if boolVal, ok := xCli.(bool); ok && boolVal {
					hasCli = true
				}
			}

			if hasCli {
				filteredMethods[method] = operation

				// Collect tags from this operation
				if tags, ok := opMap["tags"].([]interface{}); ok {
					for _, t := range tags {
						if tagStr, ok := t.(string); ok {
							usedTags[tagStr] = true
						}
					}
				}
			}
		}

		if len(filteredMethods) > 0 {
			filteredPaths[path] = filteredMethods
		}
	}

	spec["paths"] = filteredPaths

	// Filter top-level tags array to only include tags used by filtered operations
	if tagsArr, ok := spec["tags"].([]interface{}); ok {
		var filteredTags []interface{}
		for _, t := range tagsArr {
			tagMap, ok := t.(map[string]interface{})
			if !ok {
				continue
			}
			name, _ := tagMap["name"].(string)
			if usedTags[name] {
				filteredTags = append(filteredTags, t)
			}
		}
		spec["tags"] = filteredTags
	}

	// Output filtered spec
	output, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, output, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Filtered spec written to %s\n", *outputFile)
	} else {
		fmt.Println(string(output))
	}

	// Print summary to stderr
	fmt.Fprintf(os.Stderr, "\nFiltered to %d paths with x-cli extension:\n", len(filteredPaths))

	// Get sorted keys
	keys := make([]string, 0, len(filteredPaths))
	for k := range filteredPaths {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	for _, path := range keys {
		fmt.Fprintf(os.Stderr, "  %s\n", path)
	}
}
