package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

func main() {
	var (
		showVersion = flag.Bool("version", false, "Show version information")
		help        = flag.Bool("help", false, "Show help information")
	)

	flag.Parse()

	// Set up controller-runtime logging. By default discard logs entirely.
	// If DEBUG=1, write logs to ~/.kc/debug.log in dev-friendly format.
	setupControllerRuntimeLogger()

	if *help {
		showHelp()
		return
	}

	if *showVersion {
		showVersionInfo()
		return
	}

	// Run the application
	if err := ui.Run(context.Background()); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
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
				f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
				if err == nil {
					// We intentionally do not close f until process exit.
					l := crzap.New(
						crzap.UseDevMode(true),
						crzap.WriteTo(f),
					)
					ctrllog.SetLogger(l)
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
}

func showHelp() {
	fmt.Println("Kubernetes Commander (kc) - A TUI for Kubernetes")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kc [flags]")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  -version    Show version information")
	fmt.Println("  -help       Show this help message")
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
