package fileaccess

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (accessor *Accessor) resolvePath(rawPath string) (string, error) {
	if accessor.workspaceRoot == "" {
		return "", fmt.Errorf("explicit file_manifest requires workspace_root")
	}
	if err := validateRelativePath(rawPath); err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(filepath.Join(accessor.workspaceRoot, rawPath))
	if err != nil {
		return "", err
	}
	if !isWithinRoot(accessor.workspaceRoot, resolved) {
		return "", fmt.Errorf("path escapes workspace_root: %s", resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("manifest path does not exist: %s", resolved)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("manifest path is not a file: %s", resolved)
	}
	return resolved, nil
}

func isWithinRoot(root, target string) bool {
	if root == target {
		return true
	}
	prefix := root + string(filepath.Separator)
	return strings.HasPrefix(target, prefix)
}
