package storage

import (
	"strings"
	"testing"
	"time"
)

func TestParseTodos(t *testing.T) {
	input := `# Active Todos

## High Priority
- [ ] Draft LinkedIn About section {added:2026-02-01}
- [ ] Research MCP Go SDK {added:2026-01-28}

## Normal
- [ ] Review Bluesky posting best practices {added:2026-02-03}

## Someday
- [ ] Explore calendar integration {added:2026-01-20}

# Completed
- [x] Fix SPA routing {added:2026-01-15,completed:2026-02-01}
- [x] Update GitHub README {added:2026-01-14,completed:2026-01-14}
`

	tf, err := ParseTodos(input)
	if err != nil {
		t.Fatalf("ParseTodos failed: %v", err)
	}

	// Check active todos count
	if len(tf.Active) != 4 {
		t.Errorf("expected 4 active todos, got %d", len(tf.Active))
	}

	// Check completed todos count
	if len(tf.Completed) != 2 {
		t.Errorf("expected 2 completed todos, got %d", len(tf.Completed))
	}

	// Check priority assignment
	highCount := 0
	for _, todo := range tf.Active {
		if todo.Priority == PriorityHigh {
			highCount++
		}
	}
	if highCount != 2 {
		t.Errorf("expected 2 high priority todos, got %d", highCount)
	}

	// Check a specific todo
	found := false
	for _, todo := range tf.Active {
		if strings.Contains(todo.Text, "Draft LinkedIn") {
			found = true
			if todo.Priority != PriorityHigh {
				t.Errorf("expected high priority, got %s", todo.Priority)
			}
			expectedDate := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
			if !todo.Added.Equal(expectedDate) {
				t.Errorf("expected added date %v, got %v", expectedDate, todo.Added)
			}
		}
	}
	if !found {
		t.Error("expected to find 'Draft LinkedIn' todo")
	}

	// Check completed todo has both dates
	for _, todo := range tf.Completed {
		if strings.Contains(todo.Text, "Fix SPA") {
			if todo.CompletedAt == nil {
				t.Error("expected completed date to be set")
			}
		}
	}
}

func TestSerializeTodos_RoundTrip(t *testing.T) {
	input := `# Active Todos

## High Priority
- [ ] Task one {added:2026-02-01}

## Normal
- [ ] Task two {added:2026-02-02}

## Someday
- [ ] Task three {added:2026-02-03}

# Completed
- [x] Done task {added:2026-01-15,completed:2026-02-01}
`

	tf, err := ParseTodos(input)
	if err != nil {
		t.Fatalf("ParseTodos failed: %v", err)
	}

	output := SerializeTodos(tf)

	// Parse again
	tf2, err := ParseTodos(output)
	if err != nil {
		t.Fatalf("Second ParseTodos failed: %v", err)
	}

	// Verify same number of items
	if len(tf.Active) != len(tf2.Active) {
		t.Errorf("active count mismatch: %d vs %d", len(tf.Active), len(tf2.Active))
	}
	if len(tf.Completed) != len(tf2.Completed) {
		t.Errorf("completed count mismatch: %d vs %d", len(tf.Completed), len(tf2.Completed))
	}
}

func TestParseStrategy(t *testing.T) {
	input := `# Discoverability Strategy Progress

## Current Phase
Foundation (Month 1-2)

## Active Milestones
- [ ] Publish first blog post — Due: 2026-02-15 {added:2026-01-15}
- [ ] Deploy MCP server to Fly.io — Due: 2026-02-28 {added:2026-02-01}

## Completed Milestones
- [x] Site migration to Vercel {added:2026-01-20,completed:2026-02-03}
- [x] Align all professional profiles {added:2026-01-15,completed:2026-01-25}

## Notes
- First blog post published and distributed
- Good momentum on technical foundation work
`

	s, err := ParseStrategy(input)
	if err != nil {
		t.Fatalf("ParseStrategy failed: %v", err)
	}

	if s.CurrentPhase != "Foundation (Month 1-2)" {
		t.Errorf("expected phase 'Foundation (Month 1-2)', got %q", s.CurrentPhase)
	}

	if len(s.ActiveMilestones) != 2 {
		t.Errorf("expected 2 active milestones, got %d", len(s.ActiveMilestones))
	}

	if len(s.CompletedMilestones) != 2 {
		t.Errorf("expected 2 completed milestones, got %d", len(s.CompletedMilestones))
	}

	if len(s.Notes) != 2 {
		t.Errorf("expected 2 notes, got %d", len(s.Notes))
	}

	// Check due date parsing
	if s.ActiveMilestones[0].Due == nil {
		t.Error("expected due date to be parsed")
	} else {
		expectedDue := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
		if !s.ActiveMilestones[0].Due.Equal(expectedDue) {
			t.Errorf("expected due date %v, got %v", expectedDue, s.ActiveMilestones[0].Due)
		}
	}
}

func TestSerializeStrategy_RoundTrip(t *testing.T) {
	input := `# Discoverability Strategy Progress

## Current Phase
Test Phase

## Active Milestones
- [ ] Milestone one — Due: 2026-02-15 {added:2026-01-15}

## Completed Milestones
- [x] Done milestone {added:2026-01-10,completed:2026-01-20}

## Notes
- A note here
`

	s, err := ParseStrategy(input)
	if err != nil {
		t.Fatalf("ParseStrategy failed: %v", err)
	}

	output := SerializeStrategy(s)

	s2, err := ParseStrategy(output)
	if err != nil {
		t.Fatalf("Second ParseStrategy failed: %v", err)
	}

	if s.CurrentPhase != s2.CurrentPhase {
		t.Errorf("phase mismatch: %q vs %q", s.CurrentPhase, s2.CurrentPhase)
	}
	if len(s.ActiveMilestones) != len(s2.ActiveMilestones) {
		t.Errorf("active milestone count mismatch: %d vs %d", len(s.ActiveMilestones), len(s2.ActiveMilestones))
	}
}

func TestParseReadingList(t *testing.T) {
	input := `# Reading List

## To Read
- [ ] https://example.com/mcp-article — Added: 2026-02-01
- [ ] https://example.com/go-patterns — Added: 2026-01-28 — Notes: Recommended by colleague

## Read
- [x] https://example.com/other-article — Read: 2026-01-25 — Notes: Good overview of deployment patterns
`

	rl, err := ParseReadingList(input)
	if err != nil {
		t.Fatalf("ParseReadingList failed: %v", err)
	}

	if len(rl.ToRead) != 2 {
		t.Errorf("expected 2 to-read items, got %d", len(rl.ToRead))
	}

	if len(rl.Read) != 1 {
		t.Errorf("expected 1 read item, got %d", len(rl.Read))
	}

	// Check URL parsing
	if rl.ToRead[0].URL != "https://example.com/mcp-article" {
		t.Errorf("expected URL 'https://example.com/mcp-article', got %q", rl.ToRead[0].URL)
	}

	// Check notes parsing
	if rl.ToRead[1].Notes != "Recommended by colleague" {
		t.Errorf("expected notes 'Recommended by colleague', got %q", rl.ToRead[1].Notes)
	}
}

func TestSerializeReadingList_RoundTrip(t *testing.T) {
	input := `# Reading List

## To Read
- [ ] https://example.com/article1 — Added: 2026-02-01 — Notes: Test note

## Read
- [x] https://example.com/article2 — Read: 2026-01-25 — Notes: Finished
`

	rl, err := ParseReadingList(input)
	if err != nil {
		t.Fatalf("ParseReadingList failed: %v", err)
	}

	output := SerializeReadingList(rl)

	rl2, err := ParseReadingList(output)
	if err != nil {
		t.Fatalf("Second ParseReadingList failed: %v", err)
	}

	if len(rl.ToRead) != len(rl2.ToRead) {
		t.Errorf("to-read count mismatch: %d vs %d", len(rl.ToRead), len(rl2.ToRead))
	}
	if len(rl.Read) != len(rl2.Read) {
		t.Errorf("read count mismatch: %d vs %d", len(rl.Read), len(rl2.Read))
	}
}

func TestParseReminders(t *testing.T) {
	input := `# Reminders

## Upcoming
- 2026-02-10: Follow up on LinkedIn connection requests {added:2026-02-03}
- 2026-02-15: Review week 3 progress {added:2026-02-01}

## Completed
- 2026-02-01: Check site meta tags {added:2026-01-25,completed:2026-02-01}
`

	rf, err := ParseReminders(input)
	if err != nil {
		t.Fatalf("ParseReminders failed: %v", err)
	}

	if len(rf.Upcoming) != 2 {
		t.Errorf("expected 2 upcoming reminders, got %d", len(rf.Upcoming))
	}

	if len(rf.Completed) != 1 {
		t.Errorf("expected 1 completed reminder, got %d", len(rf.Completed))
	}

	// Check date parsing
	expectedDate := time.Date(2026, 2, 10, 0, 0, 0, 0, time.UTC)
	if !rf.Upcoming[0].Date.Equal(expectedDate) {
		t.Errorf("expected date %v, got %v", expectedDate, rf.Upcoming[0].Date)
	}

	// Check text extraction
	if rf.Upcoming[0].Text != "Follow up on LinkedIn connection requests" {
		t.Errorf("expected text 'Follow up on LinkedIn connection requests', got %q", rf.Upcoming[0].Text)
	}
}

func TestSerializeReminders_RoundTrip(t *testing.T) {
	input := `# Reminders

## Upcoming
- 2026-02-10: Test reminder {added:2026-02-03}

## Completed
- 2026-02-01: Done reminder {added:2026-01-25,completed:2026-02-01}
`

	rf, err := ParseReminders(input)
	if err != nil {
		t.Fatalf("ParseReminders failed: %v", err)
	}

	output := SerializeReminders(rf)

	rf2, err := ParseReminders(output)
	if err != nil {
		t.Fatalf("Second ParseReminders failed: %v", err)
	}

	if len(rf.Upcoming) != len(rf2.Upcoming) {
		t.Errorf("upcoming count mismatch: %d vs %d", len(rf.Upcoming), len(rf2.Upcoming))
	}
	if len(rf.Completed) != len(rf2.Completed) {
		t.Errorf("completed count mismatch: %d vs %d", len(rf.Completed), len(rf2.Completed))
	}
}
