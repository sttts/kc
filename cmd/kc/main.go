package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	pprof "net/http/pprof"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/alecthomas/kong"
	"github.com/go-logr/logr"
	"github.com/sttts/kc/internal/ui"
	"go.uber.org/zap/zapcore"
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
	Kubeconfig string          `help:"Path to kubeconfig file (overrides KUBECONFIG)"`
	Namespace  string          `help:"Namespace to open on startup" short:"n"`
	PprofAddr  string          `help:"Start net/http/pprof listener on this address (e.g., localhost:6060)"`
	Verbosity  int             `help:"klog verbosity level (same as --v)" name:"v"`
	Root       rootCommand     `cmd:"" hidden:"true" default:"1"`
	Get        getCommand      `cmd:"get" help:"Mirror kubectl get"`
	Logs       logsCommand     `cmd:"logs" help:"Mirror kubectl logs"`
	Version    versionCommand  `cmd:"version" help:"Show version information"`
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

type versionCommand struct{}

// cutoverWriter routes writes to stderr+file until cutover is called, after which
// only the file is used. Shared across zap, klog, and stdlib log so all loggers
// flip at the same time.
type cutoverWriter struct {
	file      io.Writer
	stderr    io.Writer
	useStderr atomic.Bool
}

func newCutoverWriter(file io.Writer, stderr io.Writer) *cutoverWriter {
	w := &cutoverWriter{file: file, stderr: stderr}
	w.useStderr.Store(true)
	return w
}

func (w *cutoverWriter) Write(p []byte) (int, error) {
	if w == nil || w.file == nil {
		return len(p), nil
	}
	if w.useStderr.Load() && w.stderr != nil {
		return io.MultiWriter(w.stderr, w.file).Write(p)
	}
	return w.file.Write(p)
}

func (w *cutoverWriter) Sync() error {
	if w == nil {
		return nil
	}
	// Best-effort sync; ignore errors to avoid noisy logs during shutdown.
	if ws, ok := w.file.(zapcore.WriteSyncer); ok {
		_ = ws.Sync()
	}
	if w.useStderr.Load() {
		if ws, ok := w.stderr.(zapcore.WriteSyncer); ok {
			_ = ws.Sync()
		}
	}
	return nil
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

	// Configure logging. Default: silent. With -v>=1: log to stderr during startup,
	// then to ~/.kc/debug.log only once the UI starts. With -v>=3: enable debug logs.
	switchToUILogger, debugLogPath := setupControllerRuntimeLogger(cli.Verbosity)
	startPprofServer(strings.TrimSpace(cli.PprofAddr))

	if commandMatches(ctx.Command(), "kc version", "version") {
		if err := runVersionCommand(context.Background()); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
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
		Namespace:          ns,
		StartupIntent:      intent,
		DebugLogPath:       debugLogPath,
		SwitchToFileLogger: switchToUILogger,
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

// setupControllerRuntimeLogger configures controller-runtime and klog logging.
// Default (verbosity 0): discard all logs. With verbosity >=1 logs are written to stderr
// during startup and ~/.kc/debug.log; once the returned switch function is invoked, logs
// are written to the file only. At verbosity >=3, debug logs are enabled.
func setupControllerRuntimeLogger(verbosity int) (func(), string) {
	// Register klog flags so we can set -v programmatically even though Kong owns CLI parsing.
	klog.InitFlags(nil)

	level := zapcore.InfoLevel
	if verbosity >= 3 {
		level = zapcore.DebugLevel
	}

	if verbosity >= 1 {
		_ = flag.Set("v", strconv.Itoa(verbosity))
		_ = flag.Set("logtostderr", "false")
		_ = flag.Set("alsologtostderr", "false")

		home, err := os.UserHomeDir()
		if err != nil {
			ctrllog.SetLogger(logr.Discard())
			klog.SetLogger(logr.Discard())
			klog.SetOutput(io.Discard)
			log.SetOutput(io.Discard)
			return nil, ""
		}

		dir := filepath.Join(home, ".kc")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			ctrllog.SetLogger(logr.Discard())
			klog.SetLogger(logr.Discard())
			klog.SetOutput(io.Discard)
			log.SetOutput(io.Discard)
			return nil, ""
		}

		fpath := filepath.Join(dir, "debug.log")
		_ = os.Remove(fpath)
		f, err := os.OpenFile(fpath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			ctrllog.SetLogger(logr.Discard())
			klog.SetLogger(logr.Discard())
			klog.SetOutput(io.Discard)
			log.SetOutput(io.Discard)
			return nil, ""
		}

		writer := newCutoverWriter(f, os.Stderr)
		startupLogger := crzap.New(
			crzap.UseDevMode(true),
			crzap.WriteTo(writer),
			crzap.Level(level),
		)
		ctrllog.SetLogger(startupLogger)
		log.SetOutput(writer)
		log.SetFlags(0)
		klog.SetOutput(writer)
		klog.SetLogger(ctrllog.Log)

		return func() {
			fileLogger := crzap.New(
				crzap.UseDevMode(true),
				crzap.WriteTo(writer),
				crzap.Level(level),
			)
			writer.useStderr.Store(false)
			ctrllog.SetLogger(fileLogger)
			log.SetOutput(writer)
			log.SetFlags(0)
			klog.SetOutput(writer)
			klog.SetLogger(ctrllog.Log)
		}, fpath
	}

	ctrllog.SetLogger(logr.Discard())
	klog.SetLogger(logr.Discard())
	klog.SetOutput(io.Discard)
	log.SetOutput(io.Discard)
	return nil, ""
}

func startPprofServer(addr string) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	go func() {
		fmt.Fprintf(os.Stderr, "pprof: listening on http://%s/debug/pprof/\n", addr)
		if err := http.ListenAndServe(addr, mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "pprof server error: %v\n", err)
		}
	}()
}

func showHelp() {
	fmt.Println("Kubernetes Commander (kc) - A TUI for Kubernetes")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  kc [flags]")
	fmt.Println("  kc version")
	fmt.Println("  kc get <resource> [name] [flags]")
	fmt.Println("  kc logs <pod> [flags]")
	fmt.Println()
	fmt.Println("Flags:")
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
