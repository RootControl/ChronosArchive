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

	// Single-session CLI flags — if -goal is set, config file is ignored.
	flagProject := flag.String("project", "", "project directory for a single session")
	flagGoal := flag.String("goal", "", "goal for a single session (bypasses -config)")
	flagName := flag.String("name", "session", "session name (used with -goal)")
	flagModel := flag.String("model", config.DefaultModel, "model (used with -goal)")
	flagMaxTurns := flag.Int("max-turns", config.DefaultMaxTurns, "max turns (used with -goal)")
	flagApproveReads := flag.Bool("approve-reads", true, "auto-approve reads (used with -goal)")
	flagApproveBash := flag.Bool("approve-bash", false, "auto-approve bash (used with -goal)")
	flagApproveWrites := flag.Bool("approve-writes", false, "auto-approve writes (used with -goal)")

	flag.Parse()

	var cfg *config.Config
	if *flagGoal != "" {
		if *flagProject == "" {
			fmt.Fprintln(os.Stderr, "error: -project is required when -goal is set")
			os.Exit(1)
		}
		cfg = &config.Config{
			Sessions: []config.SessionConfig{
				{
					Name:        *flagName,
					ProjectPath: *flagProject,
					Goal:        *flagGoal,
					Model:       *flagModel,
					MaxTurns:    *flagMaxTurns,
					ToolPermissions: config.ToolPermissions{
						AutoApproveReads:  *flagApproveReads,
						AutoApproveBash:   *flagApproveBash,
						AutoApproveWrites: *flagApproveWrites,
					},
				},
			},
		}
		if err := config.Resolve(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	} else {
		var err error
		cfg, err = config.Load(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
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

	nextID := len(sessions)
	launchSession := func(project, goal, name, model string, approveReads, approveBash, approveWrites bool) {
		tmp := &config.Config{Sessions: []config.SessionConfig{{
			Name:        name,
			ProjectPath: project,
			Goal:        goal,
			Model:       model,
			ToolPermissions: config.ToolPermissions{
				AutoApproveReads:  approveReads,
				AutoApproveBash:   approveBash,
				AutoApproveWrites: approveWrites,
			},
		}}}
		if err := config.Resolve(tmp); err != nil {
			return
		}
		id := fmt.Sprintf("s%d", nextID)
		nextID++
		s := session.New(id, tmp.Sessions[0])
		prog.Send(tui.NewSessionMsg{Session: s})
		go s.Run(ctx, &client, func(msg any) {
			prog.Send(msg)
		})
	}
	model.SetLaunch(launchSession)

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
