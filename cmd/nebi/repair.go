package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/aktech/darb/internal/localindex"
	"github.com/aktech/darb/internal/nebifile"
	"github.com/spf13/cobra"
)

var (
	repairDryRun bool
	repairPath   string
	repairYes    bool
)

var repairCmd = &cobra.Command{
	Use:   "repair",
	Short: "Scan filesystem and repair local index",
	Long: `Scan the filesystem for .nebi.toml metadata files and reconcile with the local index.

This command helps recover from situations where:
  - Environments were moved to different directories
  - The index.json was deleted or corrupted
  - Environments were manually copied between machines

Workflow:
  1. Load existing index.json (if present)
  2. Scan for .nebi.toml files in current directory, subdirectories, and global storage
  3. Compare found environments against index entries by ID
  4. Update paths for moved environments, remove stale entries

Examples:
  # Scan and repair (interactive)
  nebi repair

  # Preview changes without modifying
  nebi repair --dry-run

  # Scan specific directory
  nebi repair --path /home/user/projects

  # Auto-confirm all changes (for scripting)
  nebi repair -y`,
	Args: cobra.NoArgs,
	Run:  runRepair,
}

func init() {
	repairCmd.Flags().BoolVar(&repairDryRun, "dry-run", false, "Show what would change without modifying")
	repairCmd.Flags().StringVar(&repairPath, "path", "", "Scan specific directory instead of defaults")
	repairCmd.Flags().BoolVarP(&repairYes, "yes", "y", false, "Auto-confirm all actions")
}

// repairResult tracks the outcome of the repair operation.
type repairResult struct {
	okCount       int
	updatedCount  int
	orphanedCount int
	removedCount  int
}

// foundEnv represents an environment found during filesystem scan.
type foundEnv struct {
	path     string
	nebiFile *nebifile.NebiFile
}

// envStatus represents the reconciliation status of a found environment.
type envStatus struct {
	found       foundEnv
	indexEntry  *localindex.Entry
	status      string // "ok", "path_moved", "not_in_index"
	newPath     string // for path_moved status
	displayName string // spec_name:version_name format
}

// staleEntry represents an index entry with missing path.
type staleEntry struct {
	entry       localindex.Entry
	displayName string
}

func runRepair(cmd *cobra.Command, args []string) {
	store := localindex.NewStore()

	fmt.Println("Scanning for .nebi.toml files...")
	fmt.Println()

	// Load existing index
	idx, err := store.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load index: %v\n", err)
		osExit(1)
	}

	// Determine scan directories
	scanDirs := getScanDirectories(store)

	// Scan filesystem for .nebi.toml files
	foundEnvs := scanForNebiFiles(scanDirs)

	// Reconcile found environments with index
	statuses, staleEntries := reconcileWithIndex(foundEnvs, idx)

	// Display results
	result := displayResults(statuses, staleEntries)

	// Apply changes if not dry-run
	if !repairDryRun && (result.updatedCount > 0 || result.removedCount > 0) {
		if !repairYes {
			fmt.Print("\nApply these changes? [y/N] ")
			var response string
			fmt.Scanln(&response)
			if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
				fmt.Println("Aborted.")
				return
			}
		}

		applyChanges(store, idx, statuses, staleEntries)
		fmt.Println("\nChanges applied successfully.")
	} else if repairDryRun && (result.updatedCount > 0 || result.removedCount > 0) {
		fmt.Println("\nDry run - no changes made. Run without --dry-run to apply.")
	}
}

// getScanDirectories returns the list of directories to scan.
func getScanDirectories(store *localindex.Store) []string {
	if repairPath != "" {
		absPath, err := filepath.Abs(repairPath)
		if err != nil {
			return []string{repairPath}
		}
		return []string{absPath}
	}

	dirs := []string{}

	// Current directory
	cwd, err := os.Getwd()
	if err == nil {
		dirs = append(dirs, cwd)
	}

	// Global envs directory (~/.local/share/nebi/envs/)
	globalEnvsDir := filepath.Join(filepath.Dir(store.IndexPath()), "envs")
	if absPath, err := filepath.Abs(globalEnvsDir); err == nil {
		if info, err := os.Stat(absPath); err == nil && info.IsDir() {
			dirs = append(dirs, absPath)
		}
	}

	return dirs
}

// scanForNebiFiles walks the given directories looking for .nebi.toml files.
// Limits depth to avoid scanning too deep.
func scanForNebiFiles(dirs []string) []foundEnv {
	const maxDepth = 5
	found := []foundEnv{}
	seen := make(map[string]bool)

	for _, dir := range dirs {
		baseDepth := strings.Count(dir, string(filepath.Separator))

		filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip inaccessible paths
			}

			// Check depth limit
			currentDepth := strings.Count(path, string(filepath.Separator))
			if currentDepth-baseDepth > maxDepth {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip common directories that shouldn't contain nebi files
			if info.IsDir() {
				base := filepath.Base(path)
				if base == ".git" || base == "node_modules" || base == ".pixi" || base == "__pycache__" {
					return filepath.SkipDir
				}
				return nil
			}

			// Look for .nebi.toml files
			if info.Name() == nebifile.FileName {
				envDir := filepath.Dir(path)
				absDir, _ := filepath.Abs(envDir)

				// Skip if already seen
				if seen[absDir] {
					return nil
				}
				seen[absDir] = true

				// Read the nebi file (handles both formats)
				nf, err := nebifile.Read(envDir)
				if err == nil {
					found = append(found, foundEnv{
						path:     absDir,
						nebiFile: nf,
					})
				}
			}

			return nil
		})
	}

	return found
}

// reconcileWithIndex compares found environments against the index.
func reconcileWithIndex(foundEnvs []foundEnv, idx *localindex.Index) ([]envStatus, []staleEntry) {
	var statuses []envStatus
	var staleEntries []staleEntry

	// Create maps for quick lookup by ID and path
	idToEntry := make(map[string]*localindex.Entry)
	pathToEntry := make(map[string]*localindex.Entry)
	for i := range idx.Entries {
		entry := &idx.Entries[i]
		if entry.ID != "" {
			idToEntry[entry.ID] = entry
		}
		pathToEntry[entry.Path] = entry
	}

	// Process each found environment
	for _, fe := range foundEnvs {
		displayName := fe.nebiFile.Origin.SpecName
		if fe.nebiFile.Origin.VersionName != "" {
			displayName += ":" + fe.nebiFile.Origin.VersionName
		}

		status := envStatus{
			found:       fe,
			displayName: displayName,
		}

		// Check if this ID is in the index
		if fe.nebiFile.ID != "" {
			if entry, exists := idToEntry[fe.nebiFile.ID]; exists {
				if entry.Path == fe.path {
					// ID matches and path matches - all good
					status.status = "ok"
					status.indexEntry = entry
				} else {
					// ID matches but path differs - environment was moved
					status.status = "path_moved"
					status.indexEntry = entry
					status.newPath = fe.path
				}
			} else {
				// ID not in index - orphaned environment
				status.status = "not_in_index"
			}
		} else {
			// No ID (old format file) - check by path
			if entry, exists := pathToEntry[fe.path]; exists {
				status.status = "ok"
				status.indexEntry = entry
			} else {
				status.status = "not_in_index"
			}
		}

		statuses = append(statuses, status)
	}

	// Find stale entries (index entries whose paths no longer exist)
	for _, entry := range idx.Entries {
		if _, err := os.Stat(entry.Path); os.IsNotExist(err) {
			displayName := entry.SpecName
			if entry.VersionName != "" {
				displayName += ":" + entry.VersionName
			}
			staleEntries = append(staleEntries, staleEntry{
				entry:       entry,
				displayName: displayName,
			})
		}
	}

	return statuses, staleEntries
}

// displayResults shows the scan results with styled output.
func displayResults(statuses []envStatus, staleEntries []staleEntry) repairResult {
	result := repairResult{}

	if len(statuses) > 0 {
		fmt.Println("Found environments:")
		fmt.Println()

		for _, s := range statuses {
			switch s.status {
			case "ok":
				result.okCount++
				fmt.Printf("\u2713 %s at %s\n", s.displayName, formatRepairPath(s.found.path))
				fmt.Printf("  Status: OK\n")
			case "path_moved":
				result.updatedCount++
				fmt.Printf("\u26a0 %s at %s\n", s.displayName, formatRepairPath(s.found.path))
				fmt.Printf("  Status: Path moved from %s\n", formatRepairPath(s.indexEntry.Path))
				fmt.Printf("  Action: Will update index path\n")
			case "not_in_index":
				result.orphanedCount++
				fmt.Printf("\u2717 %s at %s\n", s.displayName, formatRepairPath(s.found.path))
				fmt.Printf("  Status: Not in index\n")
				fmt.Printf("  Action: Run 'nebi pull %s' to re-add\n", s.displayName)
			}
			fmt.Println()
		}
	} else {
		fmt.Println("No environments found in scanned directories.")
		fmt.Println()
	}

	if len(staleEntries) > 0 {
		fmt.Println("Stale index entries:")
		fmt.Println()

		for _, se := range staleEntries {
			result.removedCount++
			fmt.Printf("\u2717 %s at %s\n", se.displayName, formatRepairPath(se.entry.Path))
			fmt.Printf("  Status: Path not found\n")
			fmt.Printf("  Action: Will remove from index\n")
			fmt.Println()
		}
	}

	// Summary
	fmt.Println("Summary:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "  OK:\t%d\n", result.okCount)
	fmt.Fprintf(w, "  Path updates:\t%d\n", result.updatedCount)
	fmt.Fprintf(w, "  Orphaned:\t%d\n", result.orphanedCount)
	fmt.Fprintf(w, "  Stale (will remove):\t%d\n", result.removedCount)
	w.Flush()

	return result
}

// applyChanges modifies the index based on reconciliation results.
func applyChanges(store *localindex.Store, idx *localindex.Index, statuses []envStatus, staleEntries []staleEntry) {
	// Update paths for moved environments
	for _, s := range statuses {
		if s.status == "path_moved" && s.indexEntry != nil {
			for i := range idx.Entries {
				if idx.Entries[i].ID == s.indexEntry.ID {
					idx.Entries[i].Path = s.newPath
					break
				}
			}
		}
	}

	// Remove stale entries
	staleIDs := make(map[string]bool)
	for _, se := range staleEntries {
		staleIDs[se.entry.ID] = true
	}

	filtered := make([]localindex.Entry, 0, len(idx.Entries))
	for _, entry := range idx.Entries {
		if !staleIDs[entry.ID] {
			filtered = append(filtered, entry)
		}
	}
	idx.Entries = filtered

	// Save the updated index
	if err := store.Save(idx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save index: %v\n", err)
		osExit(1)
	}
}

// formatRepairPath abbreviates home directory in paths for display.
func formatRepairPath(path string) string {
	home, _ := os.UserHomeDir()
	if home != "" && strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}
