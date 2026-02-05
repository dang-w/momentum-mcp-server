package resources

import (
	"testing"
	"time"
)

func TestStartOfWeek(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Time
		expected string
	}{
		{
			name:     "Wednesday",
			input:    time.Date(2026, 2, 5, 15, 30, 0, 0, time.UTC), // Thursday
			expected: "2026-02-02", // Monday
		},
		{
			name:     "Monday",
			input:    time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC),
			expected: "2026-02-02",
		},
		{
			name:     "Sunday",
			input:    time.Date(2026, 2, 8, 23, 59, 0, 0, time.UTC),
			expected: "2026-02-02",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := startOfWeek(tt.input)
			if result.Format("2006-01-02") != tt.expected {
				t.Errorf("startOfWeek(%v) = %v, expected %v", tt.input, result.Format("2006-01-02"), tt.expected)
			}
		})
	}
}

func TestCalculateStreak(t *testing.T) {
	tests := []struct {
		name     string
		days     []contributionDay
		expected int
	}{
		{
			name:     "empty",
			days:     []contributionDay{},
			expected: 0,
		},
		{
			name: "three day streak",
			days: []contributionDay{
				{Date: time.Now().Format("2006-01-02"), ContributionCount: 5},
				{Date: time.Now().AddDate(0, 0, -1).Format("2006-01-02"), ContributionCount: 3},
				{Date: time.Now().AddDate(0, 0, -2).Format("2006-01-02"), ContributionCount: 1},
			},
			expected: 3,
		},
		{
			name: "streak with gap",
			days: []contributionDay{
				{Date: time.Now().Format("2006-01-02"), ContributionCount: 5},
				{Date: time.Now().AddDate(0, 0, -1).Format("2006-01-02"), ContributionCount: 0}, // gap
				{Date: time.Now().AddDate(0, 0, -2).Format("2006-01-02"), ContributionCount: 1},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateStreak(tt.days)
			if result != tt.expected {
				t.Errorf("calculateStreak() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestPrevDate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"2026-02-05", "2026-02-04"},
		{"2026-02-01", "2026-01-31"},
		{"2026-01-01", "2025-12-31"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := prevDate(tt.input)
			if result != tt.expected {
				t.Errorf("prevDate(%v) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
