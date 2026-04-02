package fileaccess

import (
	"fmt"

	"mira/pkg/contract"
)

const (
	DefaultMaxFileBytes = 128 * 1024
	MaxFiles            = 20
)

type Accessor struct {
	manifest      contract.FileManifest
	workspaceRoot string
}

func New(manifest contract.FileManifest, workspaceRoot string) (*Accessor, error) {
	root, err := resolveWorkspaceRoot(manifest, workspaceRoot)
	if err != nil {
		return nil, err
	}
	return &Accessor{manifest: manifest, workspaceRoot: root}, nil
}

func (accessor *Accessor) ReadAuthorizedFiles() ([]contract.ContextFile, []map[string]any, error) {
	if accessor.manifest.Mode == "none" {
		return []contract.ContextFile{}, []map[string]any{}, nil
	}
	if accessor.manifest.Mode != "explicit" {
		return nil, nil, fmt.Errorf("unsupported file_manifest mode: %s", accessor.manifest.Mode)
	}
	if len(accessor.manifest.Paths) > MaxFiles {
		return nil, nil, fmt.Errorf("file_manifest exceeds max file count: %d", MaxFiles)
	}
	return accessor.readFiles()
}

func (accessor *Accessor) readFiles() ([]contract.ContextFile, []map[string]any, error) {
	files := []contract.ContextFile{}
	filesRead := []map[string]any{}
	totalBytes := 0
	seen := map[string]struct{}{}
	perFileLimit := min(accessor.manifest.MaxTotalBytes, DefaultMaxFileBytes)
	for _, rawPath := range accessor.manifest.Paths {
		resolved, err := accessor.resolvePath(rawPath)
		if err != nil {
			return nil, nil, err
		}
		files, filesRead, totalBytes, err = accessor.appendFile(files, filesRead, seen, resolved, totalBytes, perFileLimit)
		if err != nil {
			return nil, nil, err
		}
	}
	return files, filesRead, nil
}
