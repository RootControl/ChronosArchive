// testloop is a standalone binary for testing a single session without the TUI.
// It prints all events to stdout and auto-approves permission requests.
//
// Usage:
//
//	export ANTHROPIC_API_KEY=sk-ant-...
//	go run ./cmd/testloop -project /tmp/test -goal "Create hello.go that prints Hello World" -model claude-opus-4-6
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/joho/godotenv"
	"github.com/chronosarchive/chronosarchive/config"
	"github.com/chronosarchive/chronosarchive/session"
)

func main() {
	_ = godotenv.Load() // load .env if present; ignore error if file doesn't exist

	project := flag.String("project", "", "project directory path (required)")
	goal := flag.String("goal", "", "goal for the session (required)")
	model := flag.String("model", config.DefaultModel, "Anthropic model")
	autoApprove := flag.Bool("auto-approve", false, "auto-approve all permission requests")
	flag.Parse()

	if *project == "" || *goal == "" {
		fmt.Fprintln(os.Stderr, "usage: testloop -project <path> -goal <text> [-model <model>] [-auto-approve]")
		os.Exit(1)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY is not set")
		os.Exit(1)
	}

	cfg := config.SessionConfig{
		Name:        "testloop",
		ProjectPath: *project,
		Goal:        *goal,
		Model:       *model,
		MaxTurns:    config.DefaultMaxTurns,
		ToolPermissions: config.ToolPermissions{
			AutoApproveReads:  true,
			AutoApproveBash:   *autoApprove,
			AutoApproveWrites: *autoApprove,
		},
	}

	s := session.New("test-0", cfg)
	client := anthropic.NewClient()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Permission handler goroutine — reads from PermCh and prompts the user
	// (or auto-approves if -auto-approve is set).
	go func() {
		for req := range s.PermCh {
			if *autoApprove {
				fmt.Printf("[perm] auto-approved %s: %s\n", req.ToolName, req.Description)
				s.RespCh <- session.PermissionResponse{Approved: true}
				continue
			}
			fmt.Printf("\n[PERMISSION REQUIRED]\nTool: %s\nAction: %s\nApprove? [y/n]: ", req.ToolName, req.Description)
			var ans string
			fmt.Scanln(&ans)
			s.RespCh <- session.PermissionResponse{Approved: ans == "y" || ans == "Y"}
		}
	}()

	// Print all events.
	send := func(msg any) {
		switch m := msg.(type) {
		case session.StateMsg:
			fmt.Printf("[state] %s\n", m.NewState)
		case session.LogMsg:
			e := m.Entry
			switch e.Kind {
			case session.LogText:
				fmt.Print(e.Text)
			case session.LogToolCall:
				fmt.Printf("\n[tool] %s: %s\n", e.ToolName, e.Text)
			case session.LogToolResult:
				fmt.Printf("[result] %s\n", e.Text)
			case session.LogPermission:
				fmt.Printf("[perm] %s\n", e.Text)
			case session.LogSystem:
				fmt.Printf("[sys] %s\n", e.Text)
			}
		case session.PermissionMsg:
			// Handled by the goroutine above.
		case session.DoneMsg:
			if m.Err != nil {
				fmt.Printf("\n[done] error: %v\n", m.Err)
			} else {
				fmt.Printf("\n[done] success\n")
			}
		}
	}

	s.Run(ctx, &client, send)
}
