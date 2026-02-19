// Package ui provides the terminal interface for friday.
package ui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	"github.com/friday/internal/types"
)

// Agent is the interface ui needs from the agent package.
type Agent interface {
	ProcessQuery(ctx context.Context, query string) (*types.AgentEvent, error)
}

// Run starts the interactive readline loop.
func Run(agent Agent) {
	styles := DefaultStyles()

	printBanner(styles)
	fmt.Println()
	fmt.Println(styles.SystemMessage.Render("  Type your query or 'help' for commands. Ctrl+C to exit."))
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	// Handle Ctrl+C gracefully.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		fmt.Println()
		fmt.Println(styles.SystemMessage.Render("  Goodbye!"))
		os.Exit(0)
	}()

	for {
		fmt.Print(styles.Prompt.Render("❯ "))

		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		query := strings.TrimSpace(line)
		if query == "" {
			continue
		}

		if handled := handleCommand(query, styles); handled {
			continue
		}

		fmt.Println()
		runQuery(agent, query, styles)
		fmt.Println()
	}
}

// RunOneShot runs a single query and exits -- used by `Friday "query"`.
func RunOneShot(agent Agent, query string) {
	styles := DefaultStyles()
	fmt.Println()
	runQuery(agent, query, styles)
	fmt.Println()
}

// runQuery executes a query against the agent and prints the result.
func runQuery(agent Agent, query string, styles Styles) {
	done := make(chan struct{})
	go runSpinner(styles, done)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	event, err := agent.ProcessQuery(ctx, query)

	close(done)
	time.Sleep(15 * time.Millisecond)
	fmt.Print("\r\033[K")

	if err != nil {
		fmt.Println(styles.ToolError.Render("  Error: " + err.Error()))
		return
	}

	printEvent(event, styles)
}

// runSpinner prints an animated spinner until done is closed.
func runSpinner(styles Styles, done chan struct{}) {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	spinStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7C3AED"))
	i := 0
	for {
		select {
		case <-done:
			return
		case <-time.After(80 * time.Millisecond):
			fmt.Printf("\r  %s  %s",
				spinStyle.Render(frames[i%len(frames)]),
				styles.StatusText.Render("Thinking..."),
			)
			i++
		}
	}
}

// printEvent renders an AgentEvent to stdout.
func printEvent(event *types.AgentEvent, styles Styles) {
	if event.Error != nil {
		fmt.Println(styles.ToolError.Render("  Error: " + event.Error.Error()))
		return
	}
	// Show agent message if present (reasoning/status).
	if event.Message != "" {
		printSection("Reasoning", event.Message, styles)
		fmt.Println()
	}

	// Tool results.
	for _, result := range event.AllResults {
		printToolResult(result, styles)
	}

	// Single tool result when AllResults is empty.
	if event.ToolResult != nil && len(event.AllResults) == 0 {
		printToolResult(*event.ToolResult, styles)
	}

	// Final answer.
	if event.FinalAnswer != "" {
		printSection("Explanation", event.FinalAnswer, styles)
	}
}

// printSection prints a labeled section with a divider.
func printSection(title, body string, styles Styles) {
	fmt.Println(styles.SectionHeader.Render("  " + title))
	fmt.Println(styles.Divider.Render("  " + strings.Repeat("─", 44)))
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		fmt.Println(styles.AssistantMessage.Render("  " + line))
	}
}

// printToolResult renders a single tool execution result.
func printToolResult(result types.ExecutionResult, styles Styles) {
	status := styles.ToolSuccess.Render("")
	if !result.Success {
		status = styles.ToolError.Render("✗")
	}

	dur := ""
	if result.Duration > 0 {
		dur = styles.ToolParams.Render(fmt.Sprintf("  %s", result.Duration.Round(time.Millisecond)))
	}

	fmt.Printf("  %s  %s%s\n",
		status,
		styles.ToolName.Render(result.Function.Name),
		dur,
	)

	if !result.Success && result.Error != "" {
		fmt.Println(styles.ToolError.Render("    " + result.Error))
		fmt.Println()
		return
	}

	if result.Output != "" {
		renderOutput(result.Output, styles)
	}

	fmt.Println()
}

// renderOutput parses tool output and renders it human-readably.
// JSON objects render as aligned key/value rows.
// Plain text renders as indented lines.
func renderOutput(raw string, styles Styles) {
	raw = strings.TrimSpace(raw)

	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &obj); err == nil {
		renderObject(obj, styles, 4)
		return
	}

	var arr []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		for i, item := range arr {
			if i > 0 {
				fmt.Println()
			}
			renderObject(item, styles, 4)
		}
		return
	}

	// Plain text fallback.
	for _, line := range strings.Split(raw, "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Println(styles.ToolOutput.Render("    " + line))
		}
	}
}

// renderObject renders a map as aligned key: value rows.
func renderObject(obj map[string]interface{}, styles Styles, indent int) {
	pad := strings.Repeat(" ", indent)

	maxLen := 0
	for k := range obj {
		if l := len(humanKey(k)); l > maxLen {
			maxLen = l
		}
	}

	for k, v := range obj {
		key := humanKey(k)
		spacing := strings.Repeat(" ", maxLen-len(key))
		fmt.Printf("%s%s%s  %s\n",
			pad,
			styles.ToolName.Render(key+spacing),
			styles.ToolParams.Render(":"),
			styles.ToolOutput.Render(renderValue(v)),
		)
	}
}

// renderValue converts a JSON value to a human-readable string.
func renderValue(v interface{}) string {
	switch val := v.(type) {
	case nil:
		return "—"
	case bool:
		if val {
			return "yes"
		}
		return "no"
	case float64:
		if val == float64(int(val)) {
			return fmt.Sprintf("%d", int(val))
		}
		return fmt.Sprintf("%.2f", val)
	case string:
		if val == "" {
			return "—"
		}
		return val
	case []interface{}:
		if len(val) == 0 {
			return "none"
		}
		parts := make([]string, 0, len(val))
		for _, item := range val {
			parts = append(parts, renderValue(item))
		}
		if len(parts) <= 5 {
			return strings.Join(parts, ", ")
		}
		return strings.Join(parts[:5], ", ") + fmt.Sprintf("  (+%d more)", len(parts)-5)
	case map[string]interface{}:
		parts := make([]string, 0, len(val))
		for k, item := range val {
			parts = append(parts, humanKey(k)+": "+renderValue(item))
		}
		return strings.Join(parts, "  |  ")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// humanKey converts snake_case / camelCase keys to Title Case words.
func humanKey(s string) string {
	var words []string
	var cur strings.Builder

	runes := []rune(s)
	for i, r := range runes {
		if r == '_' || r == '-' {
			if cur.Len() > 0 {
				words = append(words, cur.String())
				cur.Reset()
			}
		} else if i > 0 && unicode.IsUpper(r) && unicode.IsLower(runes[i-1]) {
			words = append(words, cur.String())
			cur.Reset()
			cur.WriteRune(r)
		} else {
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		words = append(words, cur.String())
	}

	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}

// printBanner prints the app banner to stdout.
func printBanner(styles Styles) {
	fmt.Println(styles.BannerTitle.Render(Banner()))
}

// handleCommand handles built-in commands. Returns true if handled.
func handleCommand(input string, styles Styles) bool {
	switch strings.ToLower(input) {
	case "exit", "quit", "q":
		fmt.Println(styles.SystemMessage.Render("  Goodbye!"))
		os.Exit(0)

	case "clear":
		fmt.Print("\033[H\033[2J")
		printBanner(styles)
		fmt.Println()

	case "help", "?":
		fmt.Println()
		fmt.Println(styles.SystemMessage.Render(
			"  Commands\n" +
				"  " + strings.Repeat("─", 44) + "\n" +
				"  help, ?       Show this help\n" +
				"  clear         Clear the screen\n" +
				"  exit, quit    Exit\n" +
				"\n" +
				"  Example queries\n" +
				"  " + strings.Repeat("─", 44) + "\n" +
				`  "Check if gRPC service on port 50051 is healthy"` + "\n" +
				`  "Analyze TCP connections on eth0"` + "\n" +
				`  "Ping google.com"` + "\n" +
				`  "Inspect network buffer settings"`,
		))
		fmt.Println()

	case "tools":
		fmt.Println()
		fmt.Println(styles.SystemMessage.Render(
			"  Available Tools\n" +
				"  " + strings.Repeat("─", 44) + "\n" +
				"  Network     ping, dns_lookup, port_scan,\n" +
				"              http_request, traceroute, netinfo\n" +
				"  TCP/gRPC    check_tcp_health, check_grpc_health,\n" +
				"              analyze_grpc_stream\n" +
				"  System      inspect_network_buffers, execute_sysctl_command\n" +
				"  Debugging   analyze_core_dump, analyze_memory_leak",
		))
		fmt.Println()

	default:
		return false
	}
	return true
}
