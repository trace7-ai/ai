package fileaccess

import (
	"fmt"
	"os"

	"mira/pkg/contract"
)

func (accessor *Accessor) appendFile(
	files []contract.ContextFile,
	filesRead []map[string]any,
	seen map[string]struct{},
	resolved string,
	totalBytes int,
	perFileLimit int,
) ([]contract.ContextFile, []map[string]any, int, error) {
	if _, ok := seen[resolved]; ok {
		return nil, nil, 0, fmt.Errorf("duplicate manifest path: %s", resolved)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, nil, 0, err
	}
	sizeBytes := int(info.Size())
	if sizeBytes > perFileLimit {
		return nil, nil, 0, fmt.Errorf("manifest file exceeds per-file limit: %s", resolved)
	}
	nextTotal := totalBytes + sizeBytes
	if nextTotal > accessor.manifest.MaxTotalBytes {
		return nil, nil, 0, fmt.Errorf("file_manifest exceeded max_total_bytes")
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, nil, 0, err
	}
	seen[resolved] = struct{}{}
	files = append(files, contract.ContextFile{Path: resolved, Content: string(content)})
	filesRead = append(filesRead, map[string]any{"path": resolved, "bytes": sizeBytes})
	return files, filesRead, nextTotal, nil
}
