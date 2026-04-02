package roles

var registry = map[string]Spec{
	"planner": {
		Name:                 "planner",
		Summary:              "Produce an execution plan with risks and validation steps.",
		DefaultContentFormat: "structured",
		SupportedContentFormats: map[string]struct{}{
			"structured": {},
		},
		RequiredKeys:    []string{"summary", "plan", "risks", "open_questions", "validation"},
		ResultExample:   "{\n  \"summary\": \"string\",\n  \"plan\": [\n    {\n      \"step\": \"string\",\n      \"why\": \"string\"\n    }\n  ],\n  \"risks\": [\n    {\n      \"severity\": \"high\",\n      \"title\": \"string\",\n      \"why\": \"string\"\n    }\n  ],\n  \"open_questions\": [\n    \"string\"\n  ],\n  \"validation\": [\n    \"string\"\n  ]\n}",
		ContextGuidance: "Use the prepared context as the primary source of truth.",
	},
	"reader": {
		Name:                 "reader",
		Summary:              "Read supplied context and return clear notes, extraction, or synthesis.",
		DefaultContentFormat: "markdown",
		SupportedContentFormats: map[string]struct{}{
			"structured": {},
			"markdown":   {},
			"text":       {},
		},
		RequiredKeys:    []string{"summary", "key_points"},
		ResultExample:   "{\n  \"summary\": \"string\",\n  \"key_points\": [\n    \"string\"\n  ],\n  \"sections\": [\n    {\n      \"title\": \"string\",\n      \"content\": \"string\"\n    }\n  ],\n  \"open_questions\": [\n    \"string\"\n  ]\n}",
		ContextGuidance: "Use the prepared context first. If the task directly references readable resources such as URLs, wiki pages, or documents, read them when needed.",
		MarkdownStyle:   "Prefer faithful reading over over-compression. Preserve headings, key bullets, and important wording when they matter.",
		StructuredStyle: "Preserve key terminology and keep the structure faithful to the source material.",
	},
	"reviewer": {
		Name:                 "reviewer",
		Summary:              "Review supplied changes for bugs, regressions, and missing checks.",
		DefaultContentFormat: "structured",
		SupportedContentFormats: map[string]struct{}{
			"structured": {},
		},
		RequiredKeys:    []string{"summary", "verdict", "findings", "open_questions"},
		ResultExample:   "{\n  \"summary\": \"string\",\n  \"verdict\": \"pass|needs_changes\",\n  \"findings\": [\n    {\n      \"severity\": \"critical|high|medium|low\",\n      \"title\": \"string\",\n      \"why\": \"string\",\n      \"evidence\": \"string\"\n    }\n  ],\n  \"open_questions\": [\n    \"string\"\n  ]\n}",
		ContextGuidance: "Use the prepared context as the primary source of truth.",
	},
}

func Get(name string) (Spec, bool) {
	spec, ok := registry[name]
	return spec, ok
}

func Names() []string {
	return []string{"planner", "reader", "reviewer"}
}
