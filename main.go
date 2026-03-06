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
	"github.com/godsfromthemachine/gilgamesh/session"
)

const version = "0.1.0"

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

	model := cfg.GetModel()
	client := llm.NewClient(model.Endpoint, model.APIKey, model.Name)
	hookReg := hooks.Load()
	sessLog := session.NewLogger()
	defer sessLog.Close()

	ag := agent.New(client, hookReg, sessLog)
	skills := gilgacontext.LoadSkills()

	fmt.Printf("\033[1mgilgamesh\033[0m v%s · %s · %s", version, cfg.ActiveModel, model.Name)

	// Show startup token overhead
	initTokens := ag.EstimateTokens()
	fmt.Printf(" · ~%d tok overhead\n", initTokens)

	if hookReg.HasHooks() {
		fmt.Printf("\033[90mhooks loaded\033[0m\n")
	}
	if len(skills) > 0 {
		fmt.Printf("\033[90m%d skills available\033[0m\n", len(skills))
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
