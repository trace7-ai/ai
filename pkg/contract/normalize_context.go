package contract

import "fmt"

func normalizeContext(raw any) (Context, error) {
	body, err := requireObject(raw, "context")
	if err != nil {
		return Context{}, err
	}
	files, err := normalizeContextFiles(body["files"])
	if err != nil {
		return Context{}, err
	}
	docs, err := normalizeContextDocs(body["docs"])
	if err != nil {
		return Context{}, err
	}
	diff, err := normalizeContextDiff(body["diff"])
	if err != nil {
		return Context{}, err
	}
	return Context{Diff: diff, Files: files, Docs: docs}, nil
}

func normalizeContextDiff(raw any) (string, error) {
	if raw == nil {
		return "", nil
	}
	value, ok := raw.(string)
	if !ok {
		return "", fmt.Errorf("context.diff must be a string")
	}
	return value, nil
}

func normalizeContextFiles(raw any) ([]ContextFile, error) {
	if raw == nil {
		return []ContextFile{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("context.files must be a list")
	}
	files := make([]ContextFile, 0, len(items))
	for index, item := range items {
		body, err := requireObject(item, fmt.Sprintf("context.files[%d]", index))
		if err != nil {
			return nil, err
		}
		path, err := requireString(body["path"], fmt.Sprintf("context.files[%d].path", index))
		if err != nil {
			return nil, err
		}
		content, err := requireString(body["content"], fmt.Sprintf("context.files[%d].content", index))
		if err != nil {
			return nil, err
		}
		file := ContextFile{Path: path, Content: content}
		if source, err := optionalString(body["source"], fmt.Sprintf("context.files[%d].source", index)); err != nil {
			return nil, err
		} else {
			file.Source = source
		}
		if title, err := optionalString(body["title"], fmt.Sprintf("context.files[%d].title", index)); err != nil {
			return nil, err
		} else {
			file.Title = title
		}
		files = append(files, file)
	}
	return files, nil
}

func normalizeContextDocs(raw any) ([]ContextDoc, error) {
	if raw == nil {
		return []ContextDoc{}, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("context.docs must be a list")
	}
	docs := make([]ContextDoc, 0, len(items))
	for index, item := range items {
		body, err := requireObject(item, fmt.Sprintf("context.docs[%d]", index))
		if err != nil {
			return nil, err
		}
		content, err := requireString(body["content"], fmt.Sprintf("context.docs[%d].content", index))
		if err != nil {
			return nil, err
		}
		doc := ContextDoc{Content: content}
		if source, err := optionalString(body["source"], fmt.Sprintf("context.docs[%d].source", index)); err != nil {
			return nil, err
		} else {
			doc.Source = source
		}
		if title, err := optionalString(body["title"], fmt.Sprintf("context.docs[%d].title", index)); err != nil {
			return nil, err
		} else {
			doc.Title = title
		}
		docs = append(docs, doc)
	}
	return docs, nil
}
