package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sschimanski/kc/internal/ui"
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

	if *help {
		showHelp()
		return
	}

	if *showVersion {
		showVersionInfo()
		return
	}

	// Run the application
	if err := ui.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
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
