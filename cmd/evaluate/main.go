// Command evaluate is an MCP stdio server for running TypeSafe Jev prompts.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// version is set by release builds via -ldflags "-X main.version=...".
var version = "dev"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok && version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
}

const instructions = `The evaluate tool runs Jev, a TypeSafe System One model that returns typed judgments and probabilities, not generated text.
- Question types: noul (probability a yes/no condition holds), choice (one option from a criteria map), score (probability-weighted position on ordered criteria levels).
- Ask one narrow judgment per question. Question ids are NOT sent to the model, so instructions must carry the full meaning.
- Put everything the judgment needs in state; prefer a JSON object with named fields, and reference nested fields with backticked paths like ` + "`ticket.messages[0].text`" + `.
- Batch independent questions over the same state into one call; they run in parallel and cannot see each other's answers.
- Include a no-match option in a choice when nothing may fit. Score levels must describe concrete situations.
- A noul near 0.5 means uncertain, not medium intensity. Confidence measures how concentrated the distribution is, not correctness.
Docs: https://docs.typesafe.ai/llms.txt`

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := newRootCmd().ExecuteContext(ctx)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "evaluate:", err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "evaluate",
		Short:   "MCP server for TypeSafe Jev prompts",
		Version: version,
		// Stdout carries the MCP protocol; main reports errors on stderr.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetVersionTemplate("{{.Version}}\n")
	mcpCmd := &cobra.Command{Use: "mcp", Short: "Run the MCP server over stdio", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return serve(cmd.Context())
	}}
	// Args+RunE, not a bare parent: cobra checks Runnable before validating args,
	// so without both `evaluate setup typo` prints help and exits 0.
	setupCmd := &cobra.Command{
		Use:   "setup",
		Short: "Register this binary with your agents",
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	setupCmd.AddCommand(
		&cobra.Command{
			Use:   "mcp",
			Short: "Register with Claude Code, Claude Desktop, and Codex",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return runMCPSetup(cmd.Context())
			},
		},
		&cobra.Command{
			Use:   "pi",
			Short: "Install the evaluate extension for pi",
			Args:  cobra.NoArgs,
			RunE: func(*cobra.Command, []string) error {
				return runPiSetup()
			},
		},
	)
	root.AddCommand(
		mcpCmd,
		setupCmd,
		&cobra.Command{Use: "update", Short: "Update evaluate to the latest release", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context())
		}},
		&cobra.Command{Use: "version", Short: "Print the version", Args: cobra.NoArgs, Run: func(*cobra.Command, []string) {
			fmt.Println(version)
		}},
	)
	return root
}

// newClient builds the HTTP client for cfg. The credential is loaded here and
// nowhere else, so there is exactly one place a key enters the process.
//
// Unlike the version this fork was taken from, the backend comes from
// JEV_PROVIDER rather than from whichever API key happens to be set. Which
// credentials are lying around in a shell should not decide where state is
// sent, or which account pays for it. See docs/upstream-baseline.md.
func newClient(cfg Config) (*Client, error) {
	key, err := loadCredential(cfg)
	if err != nil {
		return nil, err
	}
	return &Client{
		Provider:   cfg.Provider,
		Model:      cfg.Model,
		APIKey:     key,
		Timeout:    cfg.Timeout,
		MaxRetries: cfg.MaxRetries,
		HTTP:       newHTTPClient(),
		Backoff:    time.Second,
	}, nil
}

// newServer registers the tools for cfg's backend against c. Split out from
// serve so a test can drive the real registration without a transport or a
// provider.
func newServer(cfg Config, c *Client) *mcp.Server {
	s := mcp.NewServer(
		&mcp.Implementation{Name: "evaluate", Version: version},
		&mcp.ServerOptions{Instructions: instructions},
	)
	registerTools(s, cfg, c)
	return s
}

func serve(ctx context.Context) error {
	cfg, err := resolveConfig(os.LookupEnv)
	if err != nil {
		return err
	}
	c, err := newClient(cfg)
	if err != nil {
		return err
	}
	// Nothing is printed here: stdout carries the MCP protocol, and a startup
	// banner on it is a protocol error.
	return newServer(cfg, c).Run(ctx, &mcp.StdioTransport{})
}
