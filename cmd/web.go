package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/shahneil76/kubectl-inventory/pkg/web"
	"github.com/spf13/cobra"
)

var webPort int

var webCmd = &cobra.Command{
	Use:   "web",
	Short: "Launch the kubectl-inventory radar web interface",
	Long: `Scans the cluster using the current kubeconfig context, starts a local
HTTP server on the given port, and opens the Radar UI in your browser.

Examples:
  kubectl inventory web
  kubectl inventory web --port 9090
  kubectl inventory web --all-namespaces --context prod-us-east-1`,
	RunE: runWeb,
}

func init() {
	rootCmd.AddCommand(webCmd)
	webCmd.Flags().IntVar(&webPort, "port", 8989, "Local port for the web interface")
}

func runWeb(cmd *cobra.Command, args []string) error {
	fmt.Fprintln(os.Stderr, "⣾  scanning cluster resources...")

	// Default web to all-namespaces unless a specific namespace was set.
	nsFlag := cmd.InheritedFlags().Lookup("namespace")
	if nsFlag == nil || !nsFlag.Changed {
		opts.AllNamespaces = true
	}

	ctx := cmd.Context()
	inv, err := scanInventory(ctx, opts.Namespace, opts.AllNamespaces)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✔  collected %d resources\n", len(inv.Resources))

	srv := web.New(&inv, webPort)
	if err := srv.Start(); err != nil {
		return err
	}

	url := srv.URL()
	fmt.Fprintf(os.Stderr, "\n  kubectl-inventory // radar\n")
	fmt.Fprintf(os.Stderr, "  ──────────────────────────\n")
	fmt.Fprintf(os.Stderr, "  URL  : %s\n", url)
	fmt.Fprintf(os.Stderr, "  CTX  : %s\n", inv.Context)
	fmt.Fprintf(os.Stderr, "  Press Ctrl+C to stop\n\n")

	openBrowser(url)

	// Wait for SIGINT / SIGTERM
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Fprintln(os.Stderr, "\n  shutting down...")
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutCtx)
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "  (could not open browser: %v)\n", err)
	}
}
