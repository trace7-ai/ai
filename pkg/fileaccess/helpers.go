package fileaccess

import (
	"fmt"
	"os"
	"path/filepath"

	"mira/pkg/contract"
)

func resolveWorkspaceRoot(manifest contract.FileManifest, workspaceRoot string) (string, error) {
	if manifest.Mode == "none" {
		return "", nil
	}
	if workspaceRoot == "" {
		return "", fmt.Errorf("explicit file_manifest requires session.context_hint.workspace_root")
	}
	root, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("workspace_root is not a directory: %s", root)
	}
	return root, nil
}

func validateRelativePath(rawPath string) error {
	if filepath.IsAbs(rawPath) {
		return fmt.Errorf("absolute manifest paths are not allowed: %s", rawPath)
	}
	return nil
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
