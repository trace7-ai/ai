package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"mira/pkg/contract"
)

const PromptHistoryLimit = 5

const (
	journalTaskPreviewMax    = 160
	journalSummaryPreviewMax = 160
	journalCarryPreviewMax   = 320
)

type JournalEntry struct {
	Turn         int                 `json:"turn"`
	Timestamp    string              `json:"ts"`
	SessionID    string              `json:"session_id"`
	RequestID    *string             `json:"request_id,omitempty"`
	Task         string              `json:"task"`
	Summary      string              `json:"summary"`
	CarryForward *string             `json:"carry_forward,omitempty"`
	Status       string              `json:"status"`
	ContentType  *string             `json:"content_type,omitempty"`
	Tokens       contract.TokenUsage `json:"tokens"`
	ErrorCode    *string             `json:"error_code,omitempty"`
	ErrorMessage *string             `json:"error_message,omitempty"`
}

func (store *Store) AppendJournalEntry(sessionID string, entry JournalEntry) error {
	if err := requireSessionID(sessionID); err != nil {
		return err
	}
	body, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(store.journalPath(sessionID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(body, '\n')); err != nil {
		return err
	}
	return nil
}

func (store *Store) ReadJournal(sessionID string, limit int) ([]JournalEntry, error) {
	if err := requireSessionID(sessionID); err != nil {
		return nil, err
	}
	file, err := os.Open(store.journalPath(sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return []JournalEntry{}, nil
		}
		return nil, err
	}
	defer file.Close()
	return readJournalEntries(file, limit, store.journalPath(sessionID))
}

func BuildJournalEntry(request contract.Request, response contract.Response) JournalEntry {
	summary, carryForward := summarizeResponse(response)
	entry := JournalEntry{
		Turn:         responseTurn(response.Session),
		Timestamp:    toISOFormat(time.Now().UTC()),
		SessionID:    derefString(request.Session.SessionID),
		RequestID:    request.RequestID,
		Task:         request.Task,
		Summary:      summary,
		CarryForward: carryForward,
		Status:       response.Status,
		ContentType:  response.ContentType,
		Tokens:       response.TokenUsage,
	}
	if len(response.Errors) > 0 {
		entry.ErrorCode = &response.Errors[0].Code
		entry.ErrorMessage = &response.Errors[0].Message
	}
	return entry
}

func summarizeResponse(response contract.Response) (string, *string) {
	if response.Status != "ok" {
		if len(response.Errors) == 0 {
			return "request failed", nil
		}
		return response.Errors[0].Message, nil
	}
	switch value := response.Result.(type) {
	case map[string]any:
		summary := jsonPreview(value)
		if text, ok := value["summary"].(string); ok && strings.TrimSpace(text) != "" {
			summary = text
		}
		if carry, ok := value["carry_forward"].(string); ok && strings.TrimSpace(carry) != "" {
			carry = strings.TrimSpace(carry)
			return summary, &carry
		}
		return summary, nil
	case string:
		main, carry := splitCarryForward(value)
		summary := firstMeaningfulLine(main)
		if summary == "" {
			summary = truncateText(main, 240)
		}
		if summary == "" {
			summary = "empty response"
		}
		return summary, carry
	default:
		return jsonPreview(value), nil
	}
}

func splitCarryForward(text string) (string, *string) {
	parts := strings.Split(text, "\n---\n")
	if len(parts) < 2 {
		return strings.TrimSpace(text), nil
	}
	main := strings.TrimSpace(strings.Join(parts[:len(parts)-1], "\n---\n"))
	carry := strings.TrimSpace(parts[len(parts)-1])
	if carry == "" {
		return main, nil
	}
	return main, &carry
}

func firstMeaningfulLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(strings.TrimLeft(line, "#-*0123456789.> "))
		if trimmed != "" {
			return truncateText(trimmed, 240)
		}
	}
	return ""
}

func jsonPreview(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return "unserializable result"
	}
	return truncateText(string(body), 240)
}

func truncateText(text string, max int) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= max {
		return trimmed
	}
	return strings.TrimSpace(trimmed[:max]) + "..."
}

func responseTurn(payload *contract.SessionPayload) int {
	if payload == nil {
		return 0
	}
	return payload.TurnIndex
}

func JournalContextDoc(entries []JournalEntry) *contract.ContextDoc {
	if len(entries) == 0 {
		return nil
	}
	lines := []string{"Recent session history:"}
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("- Turn %d | status=%s | task=%s", entry.Turn, entry.Status, truncateText(entry.Task, journalTaskPreviewMax)))
		lines = append(lines, "  summary: "+truncateText(entry.Summary, journalSummaryPreviewMax))
		if entry.CarryForward != nil && strings.TrimSpace(*entry.CarryForward) != "" {
			lines = append(lines, "  carry_forward: "+truncateText(*entry.CarryForward, journalCarryPreviewMax))
		}
	}
	source := "session_journal"
	title := "Recent session history"
	return &contract.ContextDoc{
		Content: strings.Join(lines, "\n"),
		Source:  &source,
		Title:   &title,
	}
}

func readJournalEntries(reader io.Reader, limit int, path string) ([]JournalEntry, error) {
	entries := []JournalEntry{}
	err := visitJournalEntries(reader, path, func(entry JournalEntry) error {
		if limit > 0 && len(entries) == limit {
			copy(entries, entries[1:])
			entries[len(entries)-1] = entry
			return nil
		}
		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func visitJournalEntries(reader io.Reader, path string, visit func(JournalEntry) error) error {
	buffer := bufio.NewReader(reader)
	for {
		line, err := buffer.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return err
		}
		line = bytesTrimSpace(line)
		if len(line) > 0 {
			var entry JournalEntry
			if jsonErr := json.Unmarshal(line, &entry); jsonErr != nil {
				return fmt.Errorf("journal entry must be valid JSON: %s", path)
			}
			if visitErr := visit(entry); visitErr != nil {
				return visitErr
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

func bytesTrimSpace(line []byte) []byte {
	start := 0
	end := len(line)
	for start < end && (line[start] == ' ' || line[start] == '\n' || line[start] == '\t' || line[start] == '\r') {
		start++
	}
	for end > start && (line[end-1] == ' ' || line[end-1] == '\n' || line[end-1] == '\t' || line[end-1] == '\r') {
		end--
	}
	return line[start:end]
}
