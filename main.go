package main

import (
	"bufio"
	"fmt"
	"os"
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

		ag := agent.New(client, hookReg, sessLog, nil)
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

	model := cfg.GetModel()
	client := llm.NewClient(model.Endpoint, model.APIKey, model.Name)
	hookReg := hooks.Load()
	sessLog := session.NewLogger()
	defer sessLog.Close()

	mem := memory.NewStore(".gilgamesh/memory.json")
	mem.Load() // best-effort — missing file is fine

	ag := agent.New(client, hookReg, sessLog, mem)
	customTools := tools.LoadCustomToolDefs()
	for _, ct := range customTools {
		ag.Registry().RegisterCustom(ct)
	}
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
	if len(customTools) > 0 {
		fmt.Printf("\033[90m%d custom tools loaded\033[0m\n", len(customTools))
	}
	if len(mem.Entries) > 0 {
		fmt.Printf("\033[90m%d memories loaded\033[0m\n", len(mem.Entries))
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
			// Save conversation history for resume
			if p := sessLog.Path(); p != "" {
				if histPath, err := session.SaveHistory(p, ag.History()); err == nil && histPath != "" {
					fmt.Printf("\033[90mHistory saved: %s\033[0m\n", histPath)
				}
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
			fmt.Println("\033[90mCommands: /model, /clear, /tokens, /skills, /memory, /remember, /forget, /resume, /sessions, /session, /distill, /exit, /help\033[0m")
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
		case input == "/memory":
			fmt.Print("\033[90m" + mem.FormatList() + "\033[0m")
			continue
		case strings.HasPrefix(input, "/remember "):
			fact := strings.TrimPrefix(input, "/remember ")
			mem.Add(fact)
			if err := mem.Save(); err != nil {
				fmt.Printf("\033[31msave error: %s\033[0m\n", err)
			} else {
				fmt.Printf("\033[90mRemembered (%d total)\033[0m\n", len(mem.Entries))
			}
			continue
		case strings.HasPrefix(input, "/forget "):
			arg := strings.TrimPrefix(input, "/forget ")
			if n, err := strconv.Atoi(arg); err == nil {
				if mem.Remove(n - 1) {
					mem.Save()
					fmt.Printf("\033[90mForgot entry %d (%d remaining)\033[0m\n", n, len(mem.Entries))
				} else {
					fmt.Printf("\033[90mNo entry #%d\033[0m\n", n)
				}
			} else {
				removed := mem.RemoveByContent(arg)
				if removed > 0 {
					mem.Save()
					fmt.Printf("\033[90mForgot %d entries matching %q (%d remaining)\033[0m\n", removed, arg, len(mem.Entries))
				} else {
					fmt.Printf("\033[90mNo memories matching %q\033[0m\n", arg)
				}
			}
			continue
		case input == "/sessions":
			histories := session.ListHistories(10)
			if len(histories) == 0 {
				fmt.Println("\033[90mNo saved sessions.\033[0m")
			} else {
				fmt.Println("\033[90mRecent sessions (newest first):\033[0m")
				for i, h := range histories {
					fmt.Printf("\033[90m  %d. %s\033[0m\n", i+1, h)
				}
				fmt.Println("\033[90mUse /resume to load the most recent, or /resume <path>\033[0m")
			}
			continue
		case input == "/resume" || strings.HasPrefix(input, "/resume "):
			histPath := ""
			arg := strings.TrimPrefix(input, "/resume ")
			if arg == "/resume" || arg == "" {
				histPath = session.LatestHistory()
				if histPath == "" {
					fmt.Println("\033[90mNo saved sessions to resume.\033[0m")
					continue
				}
			} else {
				histPath = arg
			}
			history, err := session.LoadHistory(histPath)
			if err != nil {
				fmt.Printf("\033[31mFailed to load: %s\033[0m\n", err)
				continue
			}
			ag.LoadHistory(history)
			// Count user messages for context
			userMsgs := 0
			for _, m := range history {
				if m.Role == "user" {
					userMsgs++
				}
			}
			fmt.Printf("\033[90mResumed %d messages (%d user) from %s\033[0m\n",
				len(history), userMsgs, histPath)
			fmt.Printf("\033[90mEstimated context: ~%d tokens\033[0m\n", ag.EstimateTokens())
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
