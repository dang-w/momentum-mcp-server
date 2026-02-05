package storage

import (
	"regexp"
	"strings"
	"time"
)

// Priority levels for todos.
type Priority string

const (
	PriorityHigh    Priority = "high"
	PriorityNormal  Priority = "normal"
	PrioritySomeday Priority = "someday"
)

// Todo represents a single todo item.
type Todo struct {
	Text      string
	Priority  Priority
	Completed bool
	Added     time.Time
	CompletedAt *time.Time
}

// TodoFile represents the parsed contents of todos.md.
type TodoFile struct {
	Active    []Todo
	Completed []Todo
	// Raw preserves any content we don't parse (for round-trip fidelity)
	Raw string
}

// Milestone represents a strategy milestone.
type Milestone struct {
	Text        string
	Due         *time.Time
	Completed   bool
	Added       time.Time
	CompletedAt *time.Time
}

// Strategy represents the parsed contents of strategy.md.
type Strategy struct {
	CurrentPhase       string
	ActiveMilestones   []Milestone
	CompletedMilestones []Milestone
	Notes              []string
	Raw                string
}

// ReadingItem represents a reading list entry.
type ReadingItem struct {
	URL     string
	Notes   string
	Read    bool
	Added   time.Time
	ReadAt  *time.Time
}

// ReadingList represents the parsed contents of reading-list.md.
type ReadingList struct {
	ToRead []ReadingItem
	Read   []ReadingItem
	Raw    string
}

// Reminder represents a reminder entry.
type Reminder struct {
	Date        time.Time
	Text        string
	Completed   bool
	Added       time.Time
	CompletedAt *time.Time
}

// ReminderFile represents the parsed contents of reminders.md.
type ReminderFile struct {
	Upcoming  []Reminder
	Completed []Reminder
	Raw       string
}

// Date format used in markdown files.
const dateFormat = "2006-01-02"

// Regex patterns for parsing.
var (
	// Matches: - [ ] or - [x]
	checkboxPattern = regexp.MustCompile(`^-\s*\[([ xX])\]\s*(.*)$`)
	// Matches: {added:2026-01-15} or {added:2026-01-15,completed:2026-02-01}
	metadataPattern = regexp.MustCompile(`\{([^}]+)\}`)
	// Matches: — Due: 2026-02-15
	duePattern = regexp.MustCompile(`—\s*Due:\s*(\d{4}-\d{2}-\d{2})`)
	// Matches: — Added: 2026-02-01
	addedPattern = regexp.MustCompile(`—\s*Added:\s*(\d{4}-\d{2}-\d{2})`)
	// Matches: — Read: 2026-02-01
	readPattern = regexp.MustCompile(`—\s*Read:\s*(\d{4}-\d{2}-\d{2})`)
	// Matches: — Notes: some text
	notesPattern = regexp.MustCompile(`—\s*Notes:\s*(.+)$`)
	// Matches reminder line: - 2026-02-10: Description {metadata}
	reminderLinePattern = regexp.MustCompile(`^-\s*(\d{4}-\d{2}-\d{2}):\s*(.+)$`)
)

// ParseTodos parses a todos.md file content.
func ParseTodos(content string) (*TodoFile, error) {
	tf := &TodoFile{Raw: content}
	lines := strings.Split(content, "\n")

	var currentSection string
	var currentPriority Priority

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Track which section we're in
		if strings.HasPrefix(trimmed, "# ") {
			if strings.Contains(trimmed, "Active") {
				currentSection = "active"
			} else if strings.Contains(trimmed, "Completed") {
				currentSection = "completed"
			}
			continue
		}

		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.ToLower(strings.TrimPrefix(trimmed, "## "))
			switch {
			case strings.Contains(heading, "high"):
				currentPriority = PriorityHigh
			case strings.Contains(heading, "normal"):
				currentPriority = PriorityNormal
			case strings.Contains(heading, "someday"):
				currentPriority = PrioritySomeday
			}
			continue
		}

		// Parse checkbox lines
		if matches := checkboxPattern.FindStringSubmatch(trimmed); matches != nil {
			todo := parseTodoLine(matches[1], matches[2], currentPriority)

			if currentSection == "completed" || todo.Completed {
				tf.Completed = append(tf.Completed, todo)
			} else {
				tf.Active = append(tf.Active, todo)
			}
		}
	}

	return tf, nil
}

// parseTodoLine extracts todo data from a checkbox match.
func parseTodoLine(checkbox, rest string, priority Priority) Todo {
	todo := Todo{
		Completed: checkbox == "x" || checkbox == "X",
		Priority:  priority,
	}

	// Extract and remove metadata
	text := rest
	if matches := metadataPattern.FindStringSubmatch(rest); matches != nil {
		text = strings.TrimSpace(metadataPattern.ReplaceAllString(rest, ""))
		parseMetadata(matches[1], &todo.Added, &todo.CompletedAt)
	}

	todo.Text = text
	return todo
}

// parseMetadata extracts dates from metadata string like "added:2026-01-15,completed:2026-02-01".
func parseMetadata(meta string, added *time.Time, completed **time.Time) {
	parts := strings.Split(meta, ",")
	for _, part := range parts {
		kv := strings.SplitN(strings.TrimSpace(part), ":", 2)
		if len(kv) != 2 {
			continue
		}
		key, val := strings.TrimSpace(kv[0]), strings.TrimSpace(kv[1])
		if t, err := time.Parse(dateFormat, val); err == nil {
			switch key {
			case "added":
				*added = t
			case "completed":
				tc := t
				*completed = &tc
			}
		}
	}
}

// SerializeTodos converts a TodoFile back to markdown.
func SerializeTodos(tf *TodoFile) string {
	var b strings.Builder

	b.WriteString("# Active Todos\n\n")

	// Group active todos by priority
	byPriority := map[Priority][]Todo{
		PriorityHigh:    {},
		PriorityNormal:  {},
		PrioritySomeday: {},
	}
	for _, todo := range tf.Active {
		p := todo.Priority
		if p == "" {
			p = PriorityNormal
		}
		byPriority[p] = append(byPriority[p], todo)
	}

	writePrioritySection(&b, "## High Priority", byPriority[PriorityHigh])
	writePrioritySection(&b, "## Normal", byPriority[PriorityNormal])
	writePrioritySection(&b, "## Someday", byPriority[PrioritySomeday])

	b.WriteString("# Completed\n")
	for _, todo := range tf.Completed {
		b.WriteString(formatTodoLine(todo, true))
	}

	return b.String()
}

func writePrioritySection(b *strings.Builder, heading string, todos []Todo) {
	if len(todos) == 0 {
		return
	}
	b.WriteString(heading + "\n")
	for _, todo := range todos {
		b.WriteString(formatTodoLine(todo, false))
	}
	b.WriteString("\n")
}

func formatTodoLine(todo Todo, includeCompleted bool) string {
	checkbox := "[ ]"
	if todo.Completed {
		checkbox = "[x]"
	}

	meta := ""
	if !todo.Added.IsZero() {
		meta = "{added:" + todo.Added.Format(dateFormat)
		if includeCompleted && todo.CompletedAt != nil {
			meta += ",completed:" + todo.CompletedAt.Format(dateFormat)
		}
		meta += "}"
	}

	if meta != "" {
		return "- " + checkbox + " " + todo.Text + " " + meta + "\n"
	}
	return "- " + checkbox + " " + todo.Text + "\n"
}

// ParseStrategy parses a strategy.md file content.
func ParseStrategy(content string) (*Strategy, error) {
	s := &Strategy{Raw: content}
	lines := strings.Split(content, "\n")

	var currentSection string

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimPrefix(trimmed, "## ")
			switch {
			case strings.Contains(heading, "Current Phase"):
				currentSection = "phase"
			case strings.Contains(heading, "Active"):
				currentSection = "active"
			case strings.Contains(heading, "Completed"):
				currentSection = "completed"
			case strings.Contains(heading, "Notes"):
				currentSection = "notes"
			default:
				currentSection = ""
			}
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "# ") {
			continue
		}

		switch currentSection {
		case "phase":
			if s.CurrentPhase == "" {
				s.CurrentPhase = trimmed
			}
		case "active", "completed":
			if matches := checkboxPattern.FindStringSubmatch(trimmed); matches != nil {
				milestone := parseMilestoneLine(matches[1], matches[2], lines, i)
				if currentSection == "active" {
					s.ActiveMilestones = append(s.ActiveMilestones, milestone)
				} else {
					s.CompletedMilestones = append(s.CompletedMilestones, milestone)
				}
			}
		case "notes":
			if strings.HasPrefix(trimmed, "- ") {
				s.Notes = append(s.Notes, strings.TrimPrefix(trimmed, "- "))
			}
		}
	}

	return s, nil
}

func parseMilestoneLine(checkbox, rest string, lines []string, lineIndex int) Milestone {
	m := Milestone{
		Completed: checkbox == "x" || checkbox == "X",
	}

	text := rest

	// Extract due date
	if matches := duePattern.FindStringSubmatch(rest); matches != nil {
		if t, err := time.Parse(dateFormat, matches[1]); err == nil {
			m.Due = &t
		}
		text = duePattern.ReplaceAllString(text, "")
	}

	// Extract metadata
	if matches := metadataPattern.FindStringSubmatch(text); matches != nil {
		text = strings.TrimSpace(metadataPattern.ReplaceAllString(text, ""))
		parseMetadata(matches[1], &m.Added, &m.CompletedAt)
	}

	m.Text = strings.TrimSpace(text)
	return m
}

// SerializeStrategy converts a Strategy back to markdown.
func SerializeStrategy(s *Strategy) string {
	var b strings.Builder

	b.WriteString("# Discoverability Strategy Progress\n\n")
	b.WriteString("## Current Phase\n")
	b.WriteString(s.CurrentPhase + "\n\n")

	b.WriteString("## Active Milestones\n")
	for _, m := range s.ActiveMilestones {
		b.WriteString(formatMilestoneLine(m, false))
	}
	b.WriteString("\n")

	b.WriteString("## Completed Milestones\n")
	for _, m := range s.CompletedMilestones {
		b.WriteString(formatMilestoneLine(m, true))
	}
	b.WriteString("\n")

	b.WriteString("## Notes\n")
	for _, note := range s.Notes {
		b.WriteString("- " + note + "\n")
	}

	return b.String()
}

func formatMilestoneLine(m Milestone, includeCompleted bool) string {
	checkbox := "[ ]"
	if m.Completed {
		checkbox = "[x]"
	}

	line := "- " + checkbox + " " + m.Text

	if m.Due != nil {
		line += " — Due: " + m.Due.Format(dateFormat)
	}

	if !m.Added.IsZero() {
		line += " {added:" + m.Added.Format(dateFormat)
		if includeCompleted && m.CompletedAt != nil {
			line += ",completed:" + m.CompletedAt.Format(dateFormat)
		}
		line += "}"
	}

	return line + "\n"
}

// ParseReadingList parses a reading-list.md file content.
func ParseReadingList(content string) (*ReadingList, error) {
	rl := &ReadingList{Raw: content}
	lines := strings.Split(content, "\n")

	var currentSection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimPrefix(trimmed, "## ")
			switch {
			case strings.Contains(heading, "To Read"):
				currentSection = "toread"
			case heading == "Read":
				currentSection = "read"
			}
			continue
		}

		if matches := checkboxPattern.FindStringSubmatch(trimmed); matches != nil {
			item := parseReadingLine(matches[1], matches[2])
			if currentSection == "read" || item.Read {
				rl.Read = append(rl.Read, item)
			} else {
				rl.ToRead = append(rl.ToRead, item)
			}
		}
	}

	return rl, nil
}

func parseReadingLine(checkbox, rest string) ReadingItem {
	item := ReadingItem{
		Read: checkbox == "x" || checkbox == "X",
	}

	// Split by — delimiter
	parts := strings.Split(rest, "—")
	if len(parts) > 0 {
		item.URL = strings.TrimSpace(parts[0])
	}

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if matches := addedPattern.FindStringSubmatch("— " + part); matches != nil {
			if t, err := time.Parse(dateFormat, matches[1]); err == nil {
				item.Added = t
			}
		} else if matches := readPattern.FindStringSubmatch("— " + part); matches != nil {
			if t, err := time.Parse(dateFormat, matches[1]); err == nil {
				item.ReadAt = &t
			}
		} else if matches := notesPattern.FindStringSubmatch("— " + part); matches != nil {
			item.Notes = matches[1]
		} else if strings.HasPrefix(part, "Notes:") {
			item.Notes = strings.TrimSpace(strings.TrimPrefix(part, "Notes:"))
		} else if strings.HasPrefix(part, "Added:") {
			if t, err := time.Parse(dateFormat, strings.TrimSpace(strings.TrimPrefix(part, "Added:"))); err == nil {
				item.Added = t
			}
		} else if strings.HasPrefix(part, "Read:") {
			if t, err := time.Parse(dateFormat, strings.TrimSpace(strings.TrimPrefix(part, "Read:"))); err == nil {
				item.ReadAt = &t
			}
		}
	}

	return item
}

// SerializeReadingList converts a ReadingList back to markdown.
func SerializeReadingList(rl *ReadingList) string {
	var b strings.Builder

	b.WriteString("# Reading List\n\n")
	b.WriteString("## To Read\n")
	for _, item := range rl.ToRead {
		b.WriteString(formatReadingLine(item, false))
	}
	b.WriteString("\n")

	b.WriteString("## Read\n")
	for _, item := range rl.Read {
		b.WriteString(formatReadingLine(item, true))
	}

	return b.String()
}

func formatReadingLine(item ReadingItem, isRead bool) string {
	checkbox := "[ ]"
	if item.Read {
		checkbox = "[x]"
	}

	line := "- " + checkbox + " " + item.URL

	if isRead && item.ReadAt != nil {
		line += " — Read: " + item.ReadAt.Format(dateFormat)
	} else if !item.Added.IsZero() {
		line += " — Added: " + item.Added.Format(dateFormat)
	}

	if item.Notes != "" {
		line += " — Notes: " + item.Notes
	}

	return line + "\n"
}

// ParseReminders parses a reminders.md file content.
func ParseReminders(content string) (*ReminderFile, error) {
	rf := &ReminderFile{Raw: content}
	lines := strings.Split(content, "\n")

	var currentSection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "## ") {
			heading := strings.TrimPrefix(trimmed, "## ")
			switch {
			case strings.Contains(heading, "Upcoming"):
				currentSection = "upcoming"
			case strings.Contains(heading, "Completed"):
				currentSection = "completed"
			}
			continue
		}

		if matches := reminderLinePattern.FindStringSubmatch(trimmed); matches != nil {
			reminder := parseReminderLine(matches[1], matches[2])
			if currentSection == "completed" {
				reminder.Completed = true
				rf.Completed = append(rf.Completed, reminder)
			} else {
				rf.Upcoming = append(rf.Upcoming, reminder)
			}
		}
	}

	return rf, nil
}

func parseReminderLine(dateStr, rest string) Reminder {
	r := Reminder{}

	if t, err := time.Parse(dateFormat, dateStr); err == nil {
		r.Date = t
	}

	text := rest
	if matches := metadataPattern.FindStringSubmatch(rest); matches != nil {
		text = strings.TrimSpace(metadataPattern.ReplaceAllString(rest, ""))
		parseMetadata(matches[1], &r.Added, &r.CompletedAt)
	}

	r.Text = text
	return r
}

// SerializeReminders converts a ReminderFile back to markdown.
func SerializeReminders(rf *ReminderFile) string {
	var b strings.Builder

	b.WriteString("# Reminders\n\n")
	b.WriteString("## Upcoming\n")
	for _, r := range rf.Upcoming {
		b.WriteString(formatReminderLine(r, false))
	}
	b.WriteString("\n")

	b.WriteString("## Completed\n")
	for _, r := range rf.Completed {
		b.WriteString(formatReminderLine(r, true))
	}

	return b.String()
}

func formatReminderLine(r Reminder, includeCompleted bool) string {
	line := "- " + r.Date.Format(dateFormat) + ": " + r.Text

	if !r.Added.IsZero() {
		line += " {added:" + r.Added.Format(dateFormat)
		if includeCompleted && r.CompletedAt != nil {
			line += ",completed:" + r.CompletedAt.Format(dateFormat)
		}
		line += "}"
	}

	return line + "\n"
}
