package session

import (
	"strings"
	"testing"
)

func TestJournalContextDocTruncatesLongFields(t *testing.T) {
	longTask := strings.Repeat("task-", 80)
	longSummary := strings.Repeat("summary-", 60)
	longCarry := strings.Repeat("carry-", 100)
	doc := JournalContextDoc([]JournalEntry{{
		Turn:         7,
		Status:       "ok",
		Task:         longTask,
		Summary:      longSummary,
		CarryForward: &longCarry,
	}})
	if doc == nil {
		t.Fatalf("expected context doc")
	}
	if strings.Contains(doc.Content, longTask) {
		t.Fatalf("task should be truncated in journal context")
	}
	if strings.Contains(doc.Content, longSummary) {
		t.Fatalf("summary should be truncated in journal context")
	}
	if strings.Contains(doc.Content, longCarry) {
		t.Fatalf("carry_forward should be truncated in journal context")
	}
	if !strings.Contains(doc.Content, "carry_forward:") {
		t.Fatalf("missing carry_forward line")
	}
}
