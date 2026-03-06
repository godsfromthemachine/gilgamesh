package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/godsfromthemachine/gilgamesh/agent"
	"github.com/godsfromthemachine/gilgamesh/config"
	gilgacontext "github.com/godsfromthemachine/gilgamesh/context"
	"github.com/godsfromthemachine/gilgamesh/hooks"
	"github.com/godsfromthemachine/gilgamesh/llm"
	"github.com/godsfromthemachine/gilgamesh/mcp"
	"github.com/godsfromthemachine/gilgamesh/server"
	"github.com/godsfromthemachine/gilgamesh/session"
	"github.com/godsfromthemachine/gilgamesh/tools"
)

const version = "0.5.0"

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

		ag := agent.New(client, hookReg, sessLog)
		ag.Registry().Filter(cfg.AllowedTools, cfg.DeniedTools)
		registry := ag.Registry()

		srv := server.New(registry, ag, hookReg, sessLog, version)
		if err := srv.ListenAndServe(servePort); err != nil {
			fmt.Fprintf(os.Stderr, "serve error: %s\n", err)
			os.Exit(1)
		}
		return
	}

	model := cfg.GetModel()
	client := llm.NewClient(model.Endpoint, model.APIKey, model.Name)
	hookReg := hooks.Load()
	sessLog := session.NewLogger()
	defer sessLog.Close()

	ag := agent.New(client, hookReg, sessLog)
	ag.Registry().Filter(cfg.AllowedTools, cfg.DeniedTools)
	skills := gilgacontext.LoadSkills()

	fmt.Printf("\033[1mgilgamesh\033[0m v%s · %s · %s", version, cfg.ActiveModel, model.Name)

	// Show startup token overhead
	initTokens := ag.EstimateTokens()
	fmt.Printf(" · ~%d tok overhead\n", initTokens)

	if hookReg.HasHooks() {
		fmt.Printf("\033[90mhooks loaded\033[0m\n")
	}
	if len(skills) > 0 {
		builtin, custom := gilgacontext.CountSkills(skills)
		if custom > 0 {
			fmt.Printf("\033[90m%d skills available (%d built-in, %d project)\033[0m\n", len(skills), builtin, custom)
		} else {
			fmt.Printf("\033[90m%d skills available (%d built-in)\033[0m\n", len(skills), builtin)
		}
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

		start := time.Now()
		if err := ag.Run(msg); err != nil {
			fmt.Fprintf(os.Stderr, "\nerror: %s\n", err)
			os.Exit(1)
		}
		elapsed := time.Since(start)
		fmt.Printf("\n\033[90m(%s)\033[0m\n", elapsed.Round(time.Millisecond))
		return
	}

	// Interactive REPL
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		fmt.Print("\n\033[1m>\033[0m ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Slash commands
		switch {
		case input == "/exit" || input == "/quit":
			if p := sessLog.Path(); p != "" {
				fmt.Printf("\033[90mSession: %s\033[0m\n", p)
			}
			fmt.Println("Bye.")
			return
		case input == "/clear":
			ag.ClearHistory()
			fmt.Println("\033[90mContext cleared.\033[0m")
			continue
		case strings.HasPrefix(input, "/model"):
			parts := strings.Fields(input)
			if len(parts) < 2 {
				fmt.Printf("\033[90mCurrent: %s (%s)\033[0m\n", cfg.ActiveModel, model.Name)
				fmt.Println("\033[90mUsage: /model [fast|default|heavy]\033[0m")
				continue
			}
			cfg.ActiveModel = parts[1]
			model = cfg.GetModel()
			client = llm.NewClient(model.Endpoint, model.APIKey, model.Name)
			ag.SetClient(client)
			fmt.Printf("\033[90mSwitched to %s (%s)\033[0m\n", cfg.ActiveModel, model.Name)
			continue
		case input == "/help":
			fmt.Println("\033[90mCommands: /model, /clear, /tokens, /skills, /session, /distill, /exit, /help\033[0m")
			continue
		case input == "/tokens":
			fmt.Printf("\033[90mEstimated context: ~%d tokens\033[0m\n", ag.EstimateTokens())
			continue
		case input == "/skills":
			skills = gilgacontext.LoadSkills() // reload
			fmt.Print("\033[90m" + gilgacontext.ListSkills(skills) + "\033[0m")
			continue
		case input == "/session":
			if p := sessLog.Path(); p != "" {
				fmt.Printf("\033[90mLogging to: %s\033[0m\n", p)
			} else {
				fmt.Println("\033[90mNo active session log.\033[0m")
			}
			continue
		case strings.HasPrefix(input, "/distill"):
			parts := strings.Fields(input)
			if len(parts) < 2 {
				if p := sessLog.Path(); p != "" {
					summary, err := session.Distill(p)
					if err != nil {
						fmt.Printf("\033[31m%s\033[0m\n", err)
					} else {
						fmt.Printf("\033[90m%s\033[0m\n", summary)
					}
				} else {
					fmt.Println("\033[90mNo active session.\033[0m")
				}
			} else {
				summary, err := session.Distill(parts[1])
				if err != nil {
					fmt.Printf("\033[31m%s\033[0m\n", err)
				} else {
					fmt.Printf("\033[90m%s\033[0m\n", summary)
				}
			}
			continue
		default:
			// Check for skill invocation
			if strings.HasPrefix(input, "/") {
				parts := strings.SplitN(input, " ", 2)
				skillName := strings.TrimPrefix(parts[0], "/")
				skillArgs := ""
				if len(parts) > 1 {
					skillArgs = parts[1]
				}
				if skill, ok := skills[skillName]; ok {
					input = gilgacontext.FormatSkillPrompt(skill, skillArgs)
					fmt.Printf("\033[90m[skill: %s]\033[0m\n", skillName)
				} else if !strings.ContainsAny(skillName, " \t") {
					fmt.Printf("\033[90mUnknown command: /%s\033[0m\n", skillName)
					continue
				}
			}
		}

		start := time.Now()
		if err := ag.Run(input); err != nil {
			fmt.Fprintf(os.Stderr, "\n\033[31merror: %s\033[0m\n", err)
		}
		elapsed := time.Since(start)
		fmt.Printf("\033[90m(%s)\033[0m", elapsed.Round(time.Millisecond))
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
  /skills                      List available skills
  /tokens                      Show context token estimate
  /session                     Show session log path
  /distill [path]              Summarize session for skill extraction
  /exit                        Quit
  /help                        Show commands

Configuration:
  .gilgameshfile                 Project context (loaded into system prompt)
  .gilgamesh/skills/*.md         Project-local skills
  .gilgamesh/hooks.json          Tool execution hooks
  ~/.config/gilgamesh/skills/    Global skills

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
