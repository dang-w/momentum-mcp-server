// Integration test for the storage layer.
// Run with: source .env && go run cmd/integration_test/main.go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/dang-w/momentum-mcp-server/storage"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	repo := os.Getenv("GITHUB_REPO")

	if token == "" || repo == "" {
		log.Fatal("GITHUB_TOKEN and GITHUB_REPO must be set")
	}

	gs, err := storage.NewGitHubStorage(token, repo)
	if err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	ctx := context.Background()

	// Test reading each file
	files := []string{"todos.md", "strategy.md", "reading-list.md", "reminders.md"}

	for _, file := range files {
		fmt.Printf("\n=== Testing %s ===\n", file)

		content, sha, err := gs.ReadFile(ctx, file)
		if err != nil {
			log.Printf("Failed to read %s: %v", file, err)
			continue
		}

		fmt.Printf("SHA: %s\n", sha[:8]+"...")
		fmt.Printf("Content length: %d bytes\n", len(content))

		// Try parsing
		switch file {
		case "todos.md":
			tf, err := storage.ParseTodos(content)
			if err != nil {
				log.Printf("Failed to parse todos: %v", err)
			} else {
				fmt.Printf("Parsed: %d active, %d completed todos\n", len(tf.Active), len(tf.Completed))

				// Test round-trip
				serialized := storage.SerializeTodos(tf)
				tf2, _ := storage.ParseTodos(serialized)
				if len(tf.Active) == len(tf2.Active) && len(tf.Completed) == len(tf2.Completed) {
					fmt.Println("Round-trip: OK")
				} else {
					fmt.Println("Round-trip: MISMATCH!")
				}
			}

		case "strategy.md":
			s, err := storage.ParseStrategy(content)
			if err != nil {
				log.Printf("Failed to parse strategy: %v", err)
			} else {
				fmt.Printf("Parsed: phase=%q, %d active, %d completed milestones, %d notes\n",
					s.CurrentPhase, len(s.ActiveMilestones), len(s.CompletedMilestones), len(s.Notes))
			}

		case "reading-list.md":
			rl, err := storage.ParseReadingList(content)
			if err != nil {
				log.Printf("Failed to parse reading list: %v", err)
			} else {
				fmt.Printf("Parsed: %d to-read, %d read\n", len(rl.ToRead), len(rl.Read))
			}

		case "reminders.md":
			rf, err := storage.ParseReminders(content)
			if err != nil {
				log.Printf("Failed to parse reminders: %v", err)
			} else {
				fmt.Printf("Parsed: %d upcoming, %d completed\n", len(rf.Upcoming), len(rf.Completed))
			}
		}
	}

	fmt.Println("\n=== Integration test complete ===")
}
