package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/go-logr/logr"
	"github.com/sttts/kc/internal/ui"
	klog "k8s.io/klog/v2"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type cliFlags struct {
	Version    bool        `help:"Show version information"`
	Kubeconfig string      `help:"Path to kubeconfig file (overrides KUBECONFIG)"`
	Namespace  string      `help:"Namespace to open on startup" short:"n"`
	Root       rootCommand `cmd:"" hidden:"true" default:"1"`
	Get        getCommand  `cmd:"get" help:"Mirror kubectl get"`
	Logs       logsCommand `cmd:"logs" help:"Mirror kubectl logs"`
}

type rootCommand struct{}

type getCommand struct {
	Output  string   `help:"Output format (supports yaml)" short:"o"`
	Targets []string `arg:"" optional:"" name:"target" help:"Resource(s) and optional object names"`
}

type logsCommand struct {
	Container string `help:"Container name" short:"c"`
	Follow    bool   `help:"Stream logs (follow)" short:"f"`
	Pod       string `arg:"" help:"Pod name"`
}

func main() {
	var cli cliFlags
	ctx := kong.Parse(&cli,
		kong.Name("kc"),
		kong.Description("Kubernetes Commander (kc) - A TUI for Kubernetes."),
		kong.UsageOnError(),
	)

	if kcPath := strings.TrimSpace(cli.Kubeconfig); kcPath != "" {
		_ = os.Setenv("KUBECONFIG", kcPath)
	}

	// Set up controller-runtime logging. By default discard logs entirely.
	// If DEBUG=1, write logs to ~/.kc/debug.log in dev-friendly format.
	setupControllerRuntimeLogger()

	if cli.Version {
		showVersionInfo()
		return
	}

	intent, nsOverride, err := deriveStartupIntent(ctx.Command(), &cli)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	ns := strings.TrimSpace(nsOverride)
	if ns == "" {
		ns = strings.TrimSpace(cli.Namespace)
	}

	// Run the application
	runCfg := ui.RunConfig{
		Namespace:     ns,
		StartupIntent: intent,
	}
	if err := ui.Run(context.Background(), runCfg); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
}

func deriveStartupIntent(cmd string, cli *cliFlags) (ui.StartupIntent, string, error) {
	var intent ui.StartupIntent
	if cli == nil {
		return intent, "", nil
	}

	switch {
	case commandMatches(cmd, "", "kc", "kc root", "root"):
		return intent, strings.TrimSpace(cli.Namespace), nil
	case commandMatches(cmd, "kc get", "get"):
		if len(cli.Get.Targets) == 0 {
			return intent, "", errors.New("kc get requires at least one target")
		}
		getIntent, err := parseGetIntent(cli.Get.Targets, cli.Get.Output)
		if err != nil {
			return intent, "", err
		}
		ns := selectNamespace(cli.Namespace, "")
		intent.Verb = ui.KubectlVerbGet
		intent.Namespace = ns
		intent.Get = getIntent
		return intent, ns, nil
	case commandMatches(cmd, "kc logs", "logs"):
		if strings.TrimSpace(cli.Logs.Pod) == "" {
			return intent, "", errors.New("kc logs requires a pod name")
		}
		logsIntent := &ui.LogsIntent{
			Pod:       strings.TrimSpace(cli.Logs.Pod),
			Container: strings.TrimSpace(cli.Logs.Container),
			Follow:    cli.Logs.Follow,
		}
		ns := selectNamespace(cli.Namespace, "")
		intent.Verb = ui.KubectlVerbLogs
		intent.Namespace = ns
		intent.Logs = logsIntent
		return intent, ns, nil
	default:
		return intent, "", fmt.Errorf("unsupported command %q", cmd)
	}
}

func commandMatches(cmd string, options ...string) bool {
	clean := strings.TrimSpace(cmd)
	for _, opt := range options {
		opt = strings.TrimSpace(opt)
		if opt == "" {
			if clean == "" {
				return true
			}
			continue
		}
		if clean == opt || strings.HasPrefix(clean, opt+" ") {
			return true
		}
		if !strings.HasPrefix(opt, "kc ") && (clean == opt || strings.HasPrefix(clean, opt+" ")) {
			return true
		}
	}
	return false
}

func selectNamespace(global, override string) string {
	if ns := strings.TrimSpace(override); ns != "" {
		return ns
	}
	return strings.TrimSpace(global)
}

// setupControllerRuntimeLogger configures controller-runtime's global logger.
// Default: drop logs. If DEBUG=1, write to ~/.kc/debug.log.
func setupControllerRuntimeLogger() {
	if os.Getenv("DEBUG") == "1" {
		if home, err := os.UserHomeDir(); err == nil {
			dir := filepath.Join(home, ".kc")
			// Best-effort create directory and file; fallback to discard on error.
			if err := os.MkdirAll(dir, 0o700); err == nil {
				fpath := filepath.Join(dir, "debug.log")
				// recreate the log file on each start to avoid unbounded growth
				_ = os.Remove(fpath)
				f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
				if err == nil {
					// We intentionally do not close f until process exit.
					l := crzap.New(
						crzap.UseDevMode(true),
						crzap.WriteTo(f),
					)
					ctrllog.SetLogger(l)
					log.SetOutput(f)
					log.SetFlags(0)
					klog.SetOutput(f)
					// Redirect klog to the controller-runtime logger (zap)
					klog.SetLogger(ctrllog.Log)
					return
				}
			}
		}
	}
	// Fallback: discard all controller-runtime logs
	ctrllog.SetLogger(logr.Discard())
	// Redirect klog to discard as well
	klog.SetLogger(logr.Discard())
	klog.SetOutput(io.Discard)
	log.SetOutput(io.Discard)
}

func showHelp() {
	fmt.Println("Kubernetes Commander (kc) - A TUI for Kubernetes")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kc [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -version           Show version information")
	fmt.Println("  -help              Show this help message")
	fmt.Println("  -kubeconfig <path> Use the provided kubeconfig file (overrides KUBECONFIG)")
	fmt.Println("  -namespace <name>  Open the specified namespace on startup")
	fmt.Println("  -n <name>          Alias for -namespace")
	fmt.Println()
	fmt.Println("Key Bindings:")
	fmt.Println("  F1          Help")
	fmt.Println("  F2          Resource selector")
	fmt.Println("  F3          View resource")
	fmt.Println("  F4          Edit resource")
	fmt.Println("  F5          Copy")
	fmt.Println("  F6          Rename/Move")
	fmt.Println("  F7          Create namespace")
	fmt.Println("  F8          Delete resource")
	fmt.Println("  F9          Context menu")
	fmt.Println("  F10         Quit")
	fmt.Println("  Ctrl+O      Toggle terminal")
	fmt.Println("  Tab         Switch panels")
	fmt.Println("  Ctrl+C      Quit")
	fmt.Println()
	fmt.Println("Navigation:")
	fmt.Println("  ↑/↓         Navigate items")
	fmt.Println("  Enter       Enter directory/resource")
	fmt.Println("  Space       Select item")
	fmt.Println("  A           Select all")
	fmt.Println("  U           Unselect all")
	fmt.Println("  I           Invert selection")
}

func showVersionInfo() {
	fmt.Printf("Kubernetes Commander (kc) version %s\n", version)
	fmt.Printf("Commit: %s\n", commit)
	fmt.Printf("Date: %s\n", date)
}
