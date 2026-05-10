package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"q2/internal/agent"
	"q2/internal/providers"
	"q2/internal/skills"
	"q2/internal/tools"
)

func main() {
	var (
		providerName = flag.String("provider", getenv("AGENT_PROVIDER", "mock"), "provider name: mock, openai, or qwen")
		model        = flag.String("model", getenv("AGENT_MODEL", "mock-agent"), "model name")
		workspace    = flag.String("workspace", ".", "workspace root used by file and shell tools")
		maxSteps     = flag.Int("max-steps", 8, "maximum agent loop steps")
		once         = flag.String("run", "", "run one prompt and exit")
		listTools    = flag.Bool("tools", false, "list available tools")
	)
	flag.Parse()

	root, err := filepath.Abs(*workspace)
	exitIfErr(err)

	registry := tools.NewRegistry()
	exitIfErr(tools.RegisterFileTools(registry, root))
	exitIfErr(tools.RegisterShellTool(registry, root))

	if *listTools {
		for _, spec := range registry.Specs() {
			fmt.Printf("%-12s %s\n", spec.Name, spec.Description)
		}
		return
	}

	skillSet, err := skills.LoadDir(filepath.Join(root, "skills"))
	exitIfErr(err)

	provider, err := providers.New(*providerName, providers.Config{
		Model:   *model,
		APIKey:  os.Getenv("OPENAI_API_KEY"),
		BaseURL: getenv("OPENAI_BASE_URL", "https://api.openai.com/v1"),
	})
	exitIfErr(err)

	runner := agent.New(agent.Config{
		Provider:  provider,
		Tools:     registry,
		Skills:    skillSet,
		MaxSteps:  *maxSteps,
		Workspace: root,
	})

	ctx := context.Background()
	if *once != "" {
		answer, err := runner.Run(ctx, *once)
		exitIfErr(err)
		fmt.Println(answer)
		return
	}

	fmt.Println("q2-agent ready. Type /exit to quit, /tools to list tools.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		switch input {
		case "/exit", "/quit":
			return
		case "/tools":
			for _, spec := range registry.Specs() {
				fmt.Printf("%-12s %s\n", spec.Name, spec.Description)
			}
			continue
		}
		answer, err := runner.Run(ctx, input)
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		fmt.Println(answer)
	}
	exitIfErr(scanner.Err())
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func exitIfErr(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
