package routing

import "strings"

var docExtensions = map[string]struct{}{
	".md": {}, ".mdx": {}, ".txt": {}, ".rst": {}, ".pdf": {}, ".doc": {}, ".docx": {},
}

var codeExtensions = map[string]struct{}{
	".c": {}, ".cc": {}, ".cpp": {}, ".cs": {}, ".go": {}, ".java": {}, ".js": {}, ".jsx": {},
	".kt": {}, ".mjs": {}, ".php": {}, ".py": {}, ".rb": {}, ".rs": {}, ".scala": {}, ".sh": {},
	".sql": {}, ".swift": {}, ".ts": {}, ".tsx": {},
}

var reviewKeywords = []string{"review", "reviewer", "double check", "regression", "bug", "审查", "检查", "找 bug", "找bug", "回归"}
var planKeywords = []string{"plan", "planning", "roadmap", "steps", "方案", "规划", "计划", "拆解", "步骤"}
var readKeywords = []string{"read", "summarize", "summary", "extract", "explain", "translate", "读取", "阅读", "总结", "摘要", "提炼", "整理", "翻译", "解释", "返回内容", "正文"}

func InferRole(task string, diff string, filePaths []string, hasDocs bool) string {
	taskText := strings.ToLower(task)
	if strings.TrimSpace(diff) != "" {
		return "reviewer"
	}
	if hasAnyKeyword(taskText, reviewKeywords) && hasExtension(filePaths, codeExtensions) {
		return "reviewer"
	}
	if hasAnyKeyword(taskText, planKeywords) {
		return "planner"
	}
	if hasDocs || hasExtension(filePaths, docExtensions) {
		return "reader"
	}
	if strings.Contains(task, "http://") || strings.Contains(task, "https://") || hasAnyKeyword(taskText, readKeywords) {
		return "reader"
	}
	return "planner"
}

func hasAnyKeyword(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}
	return false
}

func hasExtension(paths []string, extensions map[string]struct{}) bool {
	for _, path := range paths {
		for suffix := range extensions {
			if strings.HasSuffix(strings.ToLower(path), suffix) {
				return true
			}
		}
	}
	return false
}
