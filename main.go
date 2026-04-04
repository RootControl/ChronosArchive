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
	flagApproveReads  := flag.Bool("approve-reads",   true,  "auto-approve reads (used with -goal)")
	flagApproveBash   := flag.Bool("approve-bash",    false, "auto-approve bash (used with -goal)")
	flagApproveWrites := flag.Bool("approve-writes",  false, "auto-approve writes (used with -goal)")
	flagApproveWeb    := flag.Bool("approve-web",     false, "auto-approve web_fetch (used with -goal)")
	flagApproveHTTP   := flag.Bool("approve-http",    false, "auto-approve http_request (used with -goal)")
	flagApproveFileOps := flag.Bool("approve-file-ops", false, "auto-approve create_directory/move_file/delete_file (used with -goal)")
	flagThinking       := flag.Bool("thinking",        false, "enable extended thinking (used with -goal)")
	flagThinkingBudget := flag.Int("thinking-budget",  10000, "thinking token budget (used with -thinking)")
	flagBatch          := flag.Bool("batch",           false, "submit via Anthropic Message Batches API (50% cost, single-turn, async)")

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
					Name:           *flagName,
					ProjectPath:    *flagProject,
					Goal:           *flagGoal,
					Model:          *flagModel,
					MaxTurns:       *flagMaxTurns,
					Thinking:       *flagThinking,
					ThinkingBudget: *flagThinkingBudget,
					Batch:          *flagBatch,
					ToolPermissions: config.ToolPermissions{
						AutoApproveReads:    *flagApproveReads,
						AutoApproveBash:     *flagApproveBash,
						AutoApproveWrites:   *flagApproveWrites,
						AutoApproveWebFetch: *flagApproveWeb,
						AutoApproveHTTP:     *flagApproveHTTP,
						AutoApproveFileOps:  *flagApproveFileOps,
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
	launchSession := func(opts tui.LaunchOpts) {
		tmp := &config.Config{Sessions: []config.SessionConfig{{
			Name:           opts.Name,
			ProjectPath:    opts.Project,
			Goal:           opts.Goal,
			Model:          opts.Model,
			Thinking:       opts.Thinking,
			ThinkingBudget: opts.ThinkingBudget,
			ToolPermissions: config.ToolPermissions{
				AutoApproveReads:    opts.ApproveReads,
				AutoApproveBash:     opts.ApproveBash,
				AutoApproveWrites:   opts.ApproveWrites,
				AutoApproveWebFetch: opts.ApproveWeb,
				AutoApproveHTTP:     opts.ApproveHTTP,
				AutoApproveFileOps:  opts.ApproveFileOps,
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

	model.SetRetry(func(old *session.Session) {
		s := session.New(old.ID, old.Config)
		prog.Send(tui.RetrySessionMsg{Session: s})
		go s.Run(ctx, &client, func(msg any) {
			prog.Send(msg)
		})
	})

	// Separate sessions into batch vs interactive.
	var batchSessions []*session.Session
	var normalSessions []*session.Session
	for _, s := range sessions {
		if s.Config.Batch {
			batchSessions = append(batchSessions, s)
		} else {
			normalSessions = append(normalSessions, s)
		}
	}

	// Launch interactive sessions individually.
	for _, s := range normalSessions {
		go s.Run(ctx, &client, func(msg any) {
			prog.Send(msg)
		})
	}

	// Launch all batch sessions as a single Anthropic Batch API request.
	if len(batchSessions) > 0 {
		go session.RunBatch(ctx, &client, batchSessions, func(msg any) {
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
