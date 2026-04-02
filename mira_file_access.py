from pathlib import Path

DEFAULT_MAX_TOTAL_BYTES = 512 * 1024
DEFAULT_MAX_FILE_BYTES = 128 * 1024
MAX_FILES = 20


class FileAccessor:
    def __init__(self, manifest: dict, workspace_root: str | None):
        self.manifest = manifest
        self.workspace_root = self._resolve_workspace_root(workspace_root)

    def _resolve_workspace_root(self, workspace_root: str | None) -> Path | None:
        if self.manifest["mode"] == "none":
            return None
        if not workspace_root:
            raise ValueError("explicit file_manifest requires session.context_hint.workspace_root")
        root = Path(workspace_root).expanduser().resolve()
        if not root.exists() or not root.is_dir():
            raise ValueError(f"workspace_root is not a directory: {root}")
        return root

    def _validate_relative_path(self, raw_path: str) -> Path:
        path = Path(raw_path).expanduser()
        if path.is_absolute():
            raise ValueError(f"absolute manifest paths are not allowed: {raw_path}")
        return path

    def _resolve_path(self, raw_path: str) -> Path:
        if self.workspace_root is None:
            raise ValueError("explicit file_manifest requires workspace_root")
        resolved = (self.workspace_root / self._validate_relative_path(raw_path)).resolve()
        if self.workspace_root not in resolved.parents and resolved != self.workspace_root:
            raise ValueError(f"path escapes workspace_root: {resolved}")
        if not resolved.exists():
            raise ValueError(f"manifest path does not exist: {resolved}")
        if not resolved.is_file():
            raise ValueError(f"manifest path is not a file: {resolved}")
        return resolved

    def read_authorized_files(self) -> tuple[list[dict], list[dict]]:
        if self.manifest["mode"] == "none":
            return [], []
        if self.manifest["mode"] != "explicit":
            raise ValueError(f"unsupported file_manifest mode: {self.manifest['mode']}")
        if len(self.manifest["paths"]) > MAX_FILES:
            raise ValueError(f"file_manifest exceeds max file count: {MAX_FILES}")
        files = []
        files_read = []
        total_bytes = 0
        seen_paths = set()
        per_file_limit = min(self.manifest["max_total_bytes"], DEFAULT_MAX_FILE_BYTES)
        for raw_path in self.manifest["paths"]:
            resolved = self._resolve_path(raw_path)
            resolved_str = str(resolved)
            if resolved_str in seen_paths:
                raise ValueError(f"duplicate manifest path: {resolved_str}")
            size_bytes = resolved.stat().st_size
            if size_bytes > per_file_limit:
                raise ValueError(f"manifest file exceeds per-file limit: {resolved_str}")
            total_bytes += size_bytes
            if total_bytes > self.manifest["max_total_bytes"]:
                raise ValueError("file_manifest exceeded max_total_bytes")
            content = resolved.read_bytes().decode("utf-8", errors="replace")
            files.append({"path": resolved_str, "content": content})
            files_read.append({"path": resolved_str, "bytes": size_bytes})
            seen_paths.add(resolved_str)
        return files, files_read
