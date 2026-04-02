package fileaccess

import (
	"os"
	"testing"

	"mira/pkg/contract"
)

func manifest(paths []string) contract.FileManifest {
	return contract.FileManifest{
		Mode:          "explicit",
		Paths:         paths,
		MaxTotalBytes: 512 * 1024,
		ReadOnly:      true,
	}
}

func TestRejectsAbsoluteManifestPath(t *testing.T) {
	root := t.TempDir()
	accessor, err := New(manifest([]string{root + "/demo.txt"}), root)
	if err != nil {
		t.Fatalf("new accessor: %v", err)
	}
	_, _, err = accessor.ReadAuthorizedFiles()
	if err == nil || err.Error() != "absolute manifest paths are not allowed: "+root+"/demo.txt" {
		t.Fatalf("error = %v", err)
	}
}

func TestRejectsParentEscapePath(t *testing.T) {
	parent := t.TempDir()
	root := parent + "/root"
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	accessor, err := New(manifest([]string{"../outside.txt"}), root)
	if err != nil {
		t.Fatalf("new accessor: %v", err)
	}
	_, _, err = accessor.ReadAuthorizedFiles()
	if err == nil || err.Error() != "path escapes workspace_root: "+parent+"/outside.txt" {
		t.Fatalf("error = %v", err)
	}
}
