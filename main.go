package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/godsfromthemachine/gilgamesh/agent"
	"github.com/godsfromthemachine/gilgamesh/config"
	gilgacontext "github.com/godsfromthemachine/gilgamesh/context"
	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/llm"
	"github.com/godsfromthemachine/gilgamesh/mcp"
	"github.com/godsfromthemachine/gilgamesh/memory"
	"github.com/godsfromthemachine/gilgamesh/server"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
	"github.com/godsfromthemachine/gilgamesh/ui"
)

const version = "0.6.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %s\n", err)
		os.Exit(1)
	}

	// Parse CLI args
	modelFlag := ""
	var prompt []string
	runMode := false
	mcpMode := false
	serveMode := false
	servePort := ":7777"

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-m", "--model":
			if i+1 < len(args) {
				i++
				modelFlag = args[i]
			}
		case "run":
			runMode = true
		case "mcp":
			mcpMode = true
		case "serve":
			serveMode = true
		case "completion":
			shell := "bash"
			if i+1 < len(args) {
				i++
				shell = args[i]
			}
			printCompletion(shell)
			return
		case "man":
			printManPage()
			return
		case "-p", "--port":
			if i+1 < len(args) {
				i++
				port := args[i]
				if !strings.HasPrefix(port, ":") {
					port = ":" + port
				}
				servePort = port
			}
		case "-h", "--help":
			printUsage()
			return
		case "-v", "--version":
			fmt.Printf("gilgamesh v%s\n", version)
			return
		default:
			if runMode || len(prompt) > 0 || !strings.HasPrefix(args[i], "-") {
				prompt = append(prompt, args[i])
			}
		}
	}

	if modelFlag != "" {
		cfg.ActiveModel = modelFlag
	}

	// MCP server mode: stdio JSON-RPC, no terminal output on stdout
	if mcpMode {
		hookReg := hooks.Load()
		sessLog := session.NewLogger()
		defer sessLog.Close()
		registry := tools.NewRegistry()
		for _, ct := range tools.LoadCustomToolDefs() {
			registry.RegisterCustom(ct)
		}
		registry.Filter(cfg.AllowedTools, cfg.DeniedTools)

		srv := mcp.NewServer(registry, hookReg, sessLog, version)
		if err := srv.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "mcp error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	// HTTP API server mode
	if serveMode {
		model := cfg.GetModel()
		client := llm.NewClient(model.Endpoint, model.APIKey, model.Name)
		hookReg := hooks.Load()
		sessLog := session.NewLogger()
		defer sessLog.Close()

		ag := agent.New(client, hookReg, sessLog, nil, nil)
		for _, ct := range tools.LoadCustomToolDefs() {
			ag.Registry().RegisterCustom(ct)
		}
		ag.Registry().Filter(cfg.AllowedTools, cfg.DeniedTools)
		registry := ag.Registry()

		srv := server.New(registry, ag, hookReg, sessLog, version)
		if err := srv.ListenAndServe(servePort); err != nil {
			fmt.Fprintf(os.Stderr, "serve error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	ui.Init()

	// Show config validation warnings
	for _, w := range cfg.Validate() {
		fmt.Fprintln(os.Stderr, ui.Warning("config: "+w))
	}

	model := cfg.GetModel()
	client := llm.NewClient(model.Endpoint, model.APIKey, model.Name)
	hookReg := hooks.Load()
	sessLog := session.NewLogger()
	defer sessLog.Close()

	mem := memory.NewStore(".gilgamesh/memory.json")
	mem.Load() // best-effort — missing file is fine

	// Wire UI callbacks for CLI mode
	var spinner *ui.Spinner
	var renderer *ui.StreamRenderer
	uiCb := &agent.UICallbacks{
		OnToolStart: func(name, briefArgs string) {
			label := ui.ToolName(ui.ToolIcon() + name)
			if briefArgs != "" {
				label += " → " + briefArgs
			}
			spinner = ui.NewSpinner(label)
			spinner.Start()
		},
		OnToolSuccess: func(lines int, elapsed time.Duration) {
			spinner.Stop(ui.ToolSuccess(fmt.Sprintf("  %s %d lines (%s)", ui.SuccessIcon(), lines, elapsed.Round(time.Millisecond))))
		},
		OnToolError: func(err error, elapsed time.Duration) {
			if elapsed > 0 {
				spinner.Stop(ui.ToolError(fmt.Sprintf("  %s %s (%s)", ui.ErrorIcon(), err, elapsed.Round(time.Millisecond))))
			} else {
				spinner.Stop(ui.Warning(fmt.Sprintf("  %s", err)))
			}
		},
		OnLoopDetected: func() {
			fmt.Fprintf(os.Stderr, "\n%s\n", ui.Warning("[loop detected — forcing response]"))
		},
		OnCompact: func(count, before, after int) {
			fmt.Fprintln(os.Stderr, ui.Muted(fmt.Sprintf("[compacted %d tool results: ~%d → ~%d tokens]", count, before, after)))
		},
		OnStreamStart: func() {
			renderer = ui.NewStreamRenderer()
			fmt.Print("\n")
		},
		OnStreamToken: func(token string) {
			fmt.Print(renderer.WriteToken(token))
		},
		OnStreamEnd: func() {
			fmt.Print(renderer.Flush())
			fmt.Println()
		},
	}

	ag := agent.New(client, hookReg, sessLog, mem, uiCb)
	customTools := tools.LoadCustomToolDefs()
	for _, ct := range customTools {
		ag.Registry().RegisterCustom(ct)
	}
	ag.Registry().Filter(cfg.AllowedTools, cfg.DeniedTools)
	skills := gilgacontext.LoadSkills()

	fmt.Printf("%s v%s · %s · %s", ui.Banner("gilgamesh"), version, cfg.ActiveModel, model.Name)

	// Show startup token overhead
	initTokens := ag.EstimateTokens()
	fmt.Printf(" · ~%d tok overhead\n", initTokens)

	if hookReg.HasHooks() {
		fmt.Println(ui.Muted("hooks loaded"))
	}
	if len(skills) > 0 {
		builtin, custom := gilgacontext.CountSkills(skills)
		if custom > 0 {
			fmt.Println(ui.Muted(fmt.Sprintf("%d skills available (%d built-in, %d project)", len(skills), builtin, custom)))
		} else {
			fmt.Println(ui.Muted(fmt.Sprintf("%d skills available (%d built-in)", len(skills), builtin)))
		}
	}
	if len(customTools) > 0 {
		fmt.Println(ui.Muted(fmt.Sprintf("%d custom tools loaded", len(customTools))))
	}
	if len(mem.Entries) > 0 {
		fmt.Println(ui.Muted(fmt.Sprintf("%d memories loaded", len(mem.Entries))))
	}

	// One-shot mode
	if len(prompt) > 0 {
		msg := strings.Join(prompt, " ")

		// Check if it's a skill invocation
		if strings.HasPrefix(msg, "/") {
			parts := strings.SplitN(msg, " ", 2)
			skillName := strings.TrimPrefix(parts[0], "/")
			skillArgs := ""
			if len(parts) > 1 {
				skillArgs = parts[1]
			}
			if skill, ok := skills[skillName]; ok {
				msg = gilgacontext.FormatSkillPrompt(skill, skillArgs)
			}
		}

		// Set up Ctrl+C handling for one-shot
		ctx, cancel := context.WithCancel(context.Background())
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		go func() {
			<-sigCh
			cancel()
		}()

		start := time.Now()
		if err := ag.RunWithContext(ctx, msg); err != nil {
			if ctx.Err() != nil {
				fmt.Fprintf(os.Stderr, "\n%s\n", ui.Warning("[interrupted]"))
			} else {
				fmt.Fprintln(os.Stderr, ui.FormatError(err))
			}
			signal.Stop(sigCh)
			os.Exit(1)
		}
		signal.Stop(sigCh)
		elapsed := time.Since(start)
		fmt.Printf("\n%s\n", ui.Muted(fmt.Sprintf("(%s)", elapsed.Round(time.Millisecond))))
		return
	}

	// Build command registry for interactive REPL
	cmds := ui.NewCommandRegistry()

	// --- Navigation ---
	cmds.Register(&ui.Command{
		Name: "model", Usage: "/model [name]", Category: "Navigation",
		Description: "Switch model (fast, default, heavy)",
		Handler: func(args string) bool {
			if args == "" {
				fmt.Println(ui.Muted(fmt.Sprintf("Current: %s (%s)", cfg.ActiveModel, model.Name)))
				fmt.Println(ui.Muted("Usage: /model [fast|default|heavy]"))
				return true
			}
			cfg.ActiveModel = args
			model = cfg.GetModel()
			client = llm.NewClient(model.Endpoint, model.APIKey, model.Name)
			ag.SetClient(client)
			fmt.Println(ui.Muted(fmt.Sprintf("Switched to %s (%s)", cfg.ActiveModel, model.Name)))
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "clear", Usage: "/clear", Category: "Navigation",
		Description: "Reset context",
		Handler: func(args string) bool {
			ag.ClearHistory()
			fmt.Println(ui.Muted("Context cleared."))
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "tokens", Usage: "/tokens", Category: "Navigation",
		Description: "Token estimate",
		Handler: func(args string) bool {
			fmt.Println(ui.Muted(fmt.Sprintf("Estimated context: ~%d tokens", ag.EstimateTokens())))
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "status", Usage: "/status", Category: "Navigation",
		Description: "Show model, context, tools, skills",
		Handler: func(args string) bool {
			tbl := ui.NewTable()
			tbl.AddRow("Model", fmt.Sprintf("%s (%s)", cfg.ActiveModel, model.Name))
			tbl.AddRow("Endpoint", model.Endpoint)
			tokens := ag.EstimateTokens()
			tbl.AddRow("Context", ui.Gauge(tokens, 12000, 20))
			tbl.AddRow("Tools", fmt.Sprintf("%d", len(ag.Registry().Definitions())))
			builtin, custom := gilgacontext.CountSkills(skills)
			tbl.AddRow("Skills", fmt.Sprintf("%d (%d built-in, %d project)", len(skills), builtin, custom))
			tbl.AddRow("Memories", fmt.Sprintf("%d", len(mem.Entries)))
			if p := sessLog.Path(); p != "" {
				tbl.AddRow("Session", p)
			}
			fmt.Print(tbl.Render())
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "config", Usage: "/config", Category: "Navigation",
		Description: "Show model config",
		Handler: func(args string) bool {
			fmt.Print(cfg.Format())
			return true
		},
	})

	// --- Memory ---
	cmds.Register(&ui.Command{
		Name: "remember", Usage: "/remember <fact>", Category: "Memory",
		Description: "Remember across sessions",
		Handler: func(args string) bool {
			if args == "" {
				fmt.Println(ui.Muted("Usage: /remember <fact>"))
				return true
			}
			mem.Add(args)
			if err := mem.Save(); err != nil {
				fmt.Println(ui.ToolError(fmt.Sprintf("save error: %s", err)))
			} else {
				fmt.Println(ui.Muted(fmt.Sprintf("Remembered (%d total)", len(mem.Entries))))
			}
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "forget", Usage: "/forget <n|text>", Category: "Memory",
		Description: "Forget by number or text",
		Handler: func(args string) bool {
			if args == "" {
				fmt.Println(ui.Muted("Usage: /forget <n|text>"))
				return true
			}
			if n, err := strconv.Atoi(args); err == nil {
				if mem.Remove(n - 1) {
					mem.Save()
					fmt.Println(ui.Muted(fmt.Sprintf("Forgot entry %d (%d remaining)", n, len(mem.Entries))))
				} else {
					fmt.Println(ui.Muted(fmt.Sprintf("No entry #%d", n)))
				}
			} else {
				removed := mem.RemoveByContent(args)
				if removed > 0 {
					mem.Save()
					fmt.Println(ui.Muted(fmt.Sprintf("Forgot %d entries matching %q (%d remaining)", removed, args, len(mem.Entries))))
				} else {
					fmt.Println(ui.Muted(fmt.Sprintf("No memories matching %q", args)))
				}
			}
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "memory", Usage: "/memory", Category: "Memory",
		Description: "List remembered facts",
		Handler: func(args string) bool {
			fmt.Print(ui.Muted(mem.FormatList()))
			return true
		},
	})

	// --- Sessions ---
	cmds.Register(&ui.Command{
		Name: "resume", Usage: "/resume [path]", Category: "Sessions",
		Description: "Resume previous conversation",
		Handler: func(args string) bool {
			histPath := ""
			if args == "" {
				histPath = session.LatestHistory()
				if histPath == "" {
					fmt.Println(ui.Muted("No saved sessions to resume."))
					return true
				}
			} else {
				histPath = args
			}
			history, err := session.LoadHistory(histPath)
			if err != nil {
				fmt.Println(ui.ToolError(fmt.Sprintf("Failed to load: %s", err)))
				return true
			}
			ag.LoadHistory(history)
			userMsgs := 0
			for _, m := range history {
				if m.Role == "user" {
					userMsgs++
				}
			}
			fmt.Println(ui.Muted(fmt.Sprintf("Resumed %d messages (%d user) from %s", len(history), userMsgs, histPath)))
			fmt.Println(ui.Muted(fmt.Sprintf("Estimated context: ~%d tokens", ag.EstimateTokens())))
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "sessions", Usage: "/sessions", Category: "Sessions",
		Description: "List recent sessions",
		Handler: func(args string) bool {
			histories := session.ListHistories(10)
			if len(histories) == 0 {
				fmt.Println(ui.Muted("No saved sessions."))
			} else {
				fmt.Println(ui.Muted("Recent sessions (newest first):"))
				for i, h := range histories {
					fmt.Println(ui.Muted(fmt.Sprintf("  %d. %s", i+1, h)))
				}
				fmt.Println(ui.Muted("Use /resume to load the most recent, or /resume <path>"))
			}
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "session", Usage: "/session", Category: "Sessions",
		Description: "Show session log path",
		Handler: func(args string) bool {
			if p := sessLog.Path(); p != "" {
				fmt.Println(ui.Muted(fmt.Sprintf("Logging to: %s", p)))
			} else {
				fmt.Println(ui.Muted("No active session log."))
			}
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "distill", Usage: "/distill [path]", Category: "Sessions",
		Description: "Summarize session",
		Handler: func(args string) bool {
			if args == "" {
				if p := sessLog.Path(); p != "" {
					summary, err := session.Distill(p)
					if err != nil {
						fmt.Println(ui.ToolError(err.Error()))
					} else {
						fmt.Println(ui.Muted(summary))
					}
				} else {
					fmt.Println(ui.Muted("No active session."))
				}
			} else {
				summary, err := session.Distill(args)
				if err != nil {
					fmt.Println(ui.ToolError(err.Error()))
				} else {
					fmt.Println(ui.Muted(summary))
				}
			}
			return true
		},
	})

	// --- Other ---
	cmds.Register(&ui.Command{
		Name: "skills", Usage: "/skills", Category: "Other",
		Description: "List available skills",
		Handler: func(args string) bool {
			skills = gilgacontext.LoadSkills() // reload
			fmt.Print(ui.Muted(gilgacontext.ListSkills(skills)))
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "help", Usage: "/help", Category: "Other",
		Description: "Show this help",
		Handler: func(args string) bool {
			for _, g := range cmds.ListByCategory() {
				fmt.Println(ui.Bold(g.Category))
				tbl := ui.NewTable()
				for _, c := range g.Commands {
					tbl.AddRow(c.Usage, c.Description)
				}
				fmt.Print(tbl.Render())
				fmt.Println()
			}
			return true
		},
	})
	cmds.Register(&ui.Command{
		Name: "exit", Usage: "/exit", Category: "Other",
		Description: "Quit",
		Handler: func(args string) bool {
			if p := sessLog.Path(); p != "" {
				if histPath, err := session.SaveHistory(p, ag.History()); err == nil && histPath != "" {
					fmt.Println(ui.Muted(fmt.Sprintf("History saved: %s", histPath)))
				}
				fmt.Println(ui.Muted(fmt.Sprintf("Session: %s", p)))
			}
			fmt.Println("Bye.")
			return false // exit
		},
	})
	cmds.Register(&ui.Command{
		Name: "quit", Usage: "/quit", Category: "Other",
		Description: "Quit",
		Handler: func(args string) bool {
			return cmds.Lookup("exit").Handler(args)
		},
	})

	// Interactive REPL with Ctrl+C handling
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Print("\n" + ui.Prompt())
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Try command registry first
		handled, shouldExit := cmds.Execute(input)
		if shouldExit {
			return
		}
		if handled {
			continue
		}

		// Check for skill invocation (unregistered /commands)
		if strings.HasPrefix(input, "/") {
			parts := strings.SplitN(input, " ", 2)
			skillName := strings.TrimPrefix(parts[0], "/")
			skillArgs := ""
			if len(parts) > 1 {
				skillArgs = parts[1]
			}
			if skill, ok := skills[skillName]; ok {
				input = gilgacontext.FormatSkillPrompt(skill, skillArgs)
				fmt.Println(ui.Muted(fmt.Sprintf("[skill: %s]", skillName)))
			} else if !strings.ContainsAny(skillName, " \t") {
				fmt.Println(ui.Muted(fmt.Sprintf("Unknown command: /%s", skillName)))
				continue
			}
		}

		// Set up per-request Ctrl+C cancellation
		ctx, cancel := context.WithCancel(context.Background())
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		ctrlCCount := 0
		go func() {
			for range sigCh {
				ctrlCCount++
				if ctrlCCount >= 2 {
					fmt.Fprintf(os.Stderr, "\n%s\n", ui.Warning("[force quit]"))
					os.Exit(1)
				}
				cancel()
			}
		}()

		start := time.Now()
		if err := ag.RunWithContext(ctx, input); err != nil {
			if ctx.Err() != nil {
				fmt.Fprintf(os.Stderr, "\n%s\n", ui.Warning("[interrupted]"))
			} else {
				fmt.Fprintln(os.Stderr, ui.FormatError(err))
			}
		}
		signal.Stop(sigCh)
		elapsed := time.Since(start)
		fmt.Print(ui.Muted(fmt.Sprintf("(%s)", elapsed.Round(time.Millisecond))))
	}
}

func printUsage() {
	fmt.Printf(`gilgamesh v%s - local AI coding agent & testing companion

Usage:
  gilgamesh                     Interactive REPL
  gilgamesh run "prompt"        One-shot mode
  gilgamesh run /skill [args]   Run a skill
  gilgamesh -m MODEL [...]      Select model (fast, default, heavy)
  gilgamesh mcp                 Start MCP server (stdio JSON-RPC)
  gilgamesh serve               Start HTTP API server (:7777)
  gilgamesh serve -p 8888       Custom port
  gilgamesh completion [shell]  Generate shell completions (bash, zsh, fish)

Interactive commands:
  /model [fast|default|heavy]  Switch model
  /clear                       Reset context
  /status                      Show model, context, tools, skills
  /config                      Show model configuration
  /skills                      List available skills
  /memory                      List remembered facts
  /remember <fact>             Remember a fact across sessions
  /forget <n|text>             Forget by number or matching text
  /resume [path]               Resume a previous conversation
  /sessions                    List recent saved sessions
  /tokens                      Show context token estimate
  /session                     Show session log path
  /distill [path]              Summarize session for skill extraction
  /exit                        Quit
  /help                        Show commands

Configuration:
  .gilgameshfile                 Project context (loaded into system prompt)
  .gilgamesh/memory.json         Persistent memory (managed via /remember)
  .gilgamesh/tools.json          Custom tool definitions (shell commands)
  .gilgamesh/skills/*.md         Project-local skills
  .gilgamesh/hooks.json          Tool execution hooks
  ~/.config/gilgamesh/skills/    Global skills

Environment variables:
  GILGAMESH_ACTIVE_MODEL         Override active model profile
  GILGAMESH_ENDPOINT             Override endpoint URL
  GILGAMESH_API_KEY              Override API key
  GILGAMESH_MODEL_NAME           Override model name

Part of the Gods from the Machine project.
https://github.com/godsfromthemachine
`, version)
}

func printCompletion(shell string) {
	switch shell {
	case "bash":
		fmt.Print(`_gilgamesh() {
    local cur prev cmds
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    cmds="run mcp serve completion"

    case "$prev" in
        gilgamesh)
            COMPREPLY=( $(compgen -W "$cmds -m --model -h --help -v --version" -- "$cur") )
            return 0
            ;;
        -m|--model)
            COMPREPLY=( $(compgen -W "fast default heavy" -- "$cur") )
            return 0
            ;;
        -p|--port)
            return 0
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish" -- "$cur") )
            return 0
            ;;
        run)
            return 0
            ;;
    esac
}
complete -F _gilgamesh gilgamesh
`)
	case "zsh":
		fmt.Print(`#compdef gilgamesh

_gilgamesh() {
    local -a commands
    commands=(
        'run:One-shot mode'
        'mcp:Start MCP server'
        'serve:Start HTTP API server'
        'completion:Generate shell completions'
    )

    _arguments -C \
        '-m[Select model]:model:(fast default heavy)' \
        '--model[Select model]:model:(fast default heavy)' \
        '-h[Show help]' \
        '--help[Show help]' \
        '-v[Show version]' \
        '--version[Show version]' \
        '1:command:->cmd' \
        '*::arg:->args'

    case "$state" in
        cmd)
            _describe 'command' commands
            ;;
        args)
            case "$words[1]" in
                serve)
                    _arguments '-p[Port]:port:'
                    ;;
                completion)
                    _values 'shell' bash zsh fish
                    ;;
            esac
            ;;
    esac
}

_gilgamesh "$@"
`)
	case "fish":
		fmt.Print(`complete -c gilgamesh -n '__fish_use_subcommand' -a 'run' -d 'One-shot mode'
complete -c gilgamesh -n '__fish_use_subcommand' -a 'mcp' -d 'Start MCP server'
complete -c gilgamesh -n '__fish_use_subcommand' -a 'serve' -d 'Start HTTP API server'
complete -c gilgamesh -n '__fish_use_subcommand' -a 'completion' -d 'Generate shell completions'
complete -c gilgamesh -n '__fish_use_subcommand' -s m -l model -xa 'fast default heavy' -d 'Select model'
complete -c gilgamesh -n '__fish_use_subcommand' -s h -l help -d 'Show help'
complete -c gilgamesh -n '__fish_use_subcommand' -s v -l version -d 'Show version'
complete -c gilgamesh -n '__fish_seen_subcommand_from serve' -s p -l port -d 'Server port'
complete -c gilgamesh -n '__fish_seen_subcommand_from completion' -xa 'bash zsh fish'
`)
	default:
		fmt.Fprintf(os.Stderr, "unsupported shell: %s (supported: bash, zsh, fish)\n", shell)
		os.Exit(1)
	}
}

func printManPage() {
	fmt.Printf(`.TH GILGAMESH 1 "March 2026" "v%s" "User Commands"
.SH NAME
gilgamesh \- local AI coding agent & testing companion
.SH SYNOPSIS
.B gilgamesh
[\fIOPTIONS\fR] [\fICOMMAND\fR] [\fIARGS\fR]
.SH DESCRIPTION
Gilgamesh is a TDD-driven CLI coding agent that connects to local AI models
(llama.cpp or any OpenAI-compatible endpoint) and provides tool-calling
capabilities for software engineering tasks. It promotes a test-driven
development approach and is designed for CPU inference with small models.
.PP
Part of the Gods from the Machine project.
.SH COMMANDS
.TP
.B run \fIPROMPT\fR
One-shot mode: execute a single prompt and exit.
.TP
.B run /\fISKILL\fR [\fIARGS\fR]
Run a built-in or project-local skill.
.TP
.B mcp
Start MCP server (JSON-RPC 2.0 over stdio).
.TP
.B serve
Start HTTP API server on port 7777.
.TP
.B completion \fISHELL\fR
Generate shell completions (bash, zsh, fish).
.TP
.B man
Print this man page to stdout.
.SH OPTIONS
.TP
.B \-m, \-\-model \fINAME\fR
Select model profile (fast, default, heavy).
.TP
.B \-p, \-\-port \fIPORT\fR
Server port (with serve command).
.TP
.B \-h, \-\-help
Show usage information.
.TP
.B \-v, \-\-version
Show version.
.SH INTERACTIVE COMMANDS
.TP
.B /model [\fINAME\fR]
Switch model mid-session (fast, default, heavy).
.TP
.B /clear
Reset conversation context.
.TP
.B /status
Show model, context gauge, tools, skills, memory, session.
.TP
.B /config
Show model configuration with all profiles.
.TP
.B /skills
List available skills (built-in and project-local).
.TP
.B /memory
List remembered facts.
.TP
.B /remember \fIFACT\fR
Remember a fact across sessions.
.TP
.B /forget \fIN\fR|\fITEXT\fR
Forget by entry number or matching text.
.TP
.B /resume [\fIPATH\fR]
Resume a previous conversation.
.TP
.B /sessions
List recent saved sessions.
.TP
.B /tokens
Show estimated context token count.
.TP
.B /session
Show session log file path.
.TP
.B /distill [\fIPATH\fR]
Summarize a session for skill extraction.
.TP
.B /tdd \fIFEATURE\fR
Red-green-refactor workflow.
.TP
.B /exit
Save history and quit.
.SH FILES
.TP
.B gilgamesh.json
Model configuration (profiles, endpoints, API keys).
.TP
.B .gilgameshfile
Project context loaded into the system prompt.
.TP
.B .gilgamesh/memory.json
Persistent memory entries managed via /remember.
.TP
.B .gilgamesh/tools.json
Custom tool definitions (shell commands).
.TP
.B .gilgamesh/skills/*.md
Project-local skill templates.
.TP
.B .gilgamesh/hooks.json
Pre/post tool execution hooks.
.TP
.B .gilgamesh/sessions/
Session logs (JSONL) and conversation histories (JSON).
.TP
.B ~/.config/gilgamesh/skills/
Global skill templates.
.SH BUILT-IN SKILLS
commit, review, explain, fix, refactor, doc, tdd
.SH BUILT-IN TOOLS
read, write, edit, bash, grep, glob, test
.SH ENVIRONMENT
.TP
.B GILGAMESH_ACTIVE_MODEL
Override active model profile (e.g. fast, default, heavy).
.TP
.B GILGAMESH_ENDPOINT
Override endpoint URL for the active model.
.TP
.B GILGAMESH_API_KEY
Override API key for the active model.
.TP
.B GILGAMESH_MODEL_NAME
Override model name for the active model.
.TP
.B GILGAMESH_ARGS
Set by custom tools: full JSON arguments.
.TP
.B GILGAMESH_<PARAM>
Set by custom tools: individual parameter values.
.SH EXAMPLES
.TP
Interactive mode:
.B gilgamesh
.TP
One-shot with heavy model:
.B gilgamesh -m heavy run "refactor main.go"
.TP
TDD workflow:
.B gilgamesh run /tdd "add user validation"
.TP
MCP server for Claude Desktop:
.B gilgamesh mcp
.TP
Install man page:
.B gilgamesh man | sudo tee /usr/local/share/man/man1/gilgamesh.1
.SH SEE ALSO
.UR https://github.com/godsfromthemachine/gilgamesh
GitHub repository
.UE
.SH AUTHORS
Gods from the Machine project.
`, version)
}
