package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/joho/godotenv"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/chronosarchive/chronosarchive/config"
	"github.com/chronosarchive/chronosarchive/session"
	"github.com/chronosarchive/chronosarchive/tui"
)

func main() {
	_ = godotenv.Load() // load .env if present; ignore error if file doesn't exist

	configPath := flag.String("config", "sessions.yaml", "path to sessions config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "error: ANTHROPIC_API_KEY environment variable is not set")
		os.Exit(1)
	}

	client := anthropic.NewClient()

	// Build sessions.
	sessions := make([]*session.Session, len(cfg.Sessions))
	for i, sc := range cfg.Sessions {
		sessions[i] = session.New(fmt.Sprintf("s%d", i), sc)
	}

	// Build the TUI model.
	model := tui.NewModel(sessions)

	// Create the Bubble Tea program.
	prog := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	// Inject the program reference so the TUI can send permission responses.
	model.SetProgram(prog)

	// Launch all session goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, s := range sessions {
		s := s // capture loop variable
		go s.Run(ctx, &client, func(msg any) {
			prog.Send(msg)
		})
	}

	// Run the TUI — blocks until the user quits.
	if _, err := prog.Run(); err != nil {
		log.Fatal(err)
	}

	// Cancel all running sessions when the user quits.
	cancel()
}
