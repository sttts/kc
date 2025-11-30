# Terminal Signals Design

## Goal
Make ctrl-based signals (SIGINT/SIGTSTP) behave like a normal TTY for all embedded terminals: the global 2-line terminal and command widgets. Ctrl+C/Z should hit the foreground process group (shell + child), not just the shell line editor.

## Constraints & Observations
- Bubbleterm currently puts the PTY into raw mode, which disables ISIG; the kernel no longer synthesizes SIGINT/SIGTSTP from `^C/^Z`. Ctrl+C only clears zsh’s prompt; `sleep 10` ignores it, and Ctrl+Z never suspends.
- Bubbleterm already offers opt-in translation (`WithCtrlCSignal/WithCtrlZSignal`) that calls `SendSignal` to the process group. That works even in raw mode but only when we route keys to the focused terminal.
- Kc wraps Bubble Tea key messages (`KeyPressMsg`/`KeyReleaseMsg`); we must pass ctrl keys through untouched so bubbleterm’s detection triggers.
- We can’t disable raw mode globally, but we can re-enable ISIG (and optionally ICANON) on the PTY slave after allocation to restore kernel signal generation.

## Desired Behavior
- Ctrl+C: deliver SIGINT to the foreground process group of the PTY (shell and its child). Should stop `sleep 10` etc.
- Ctrl+Z: deliver SIGTSTP to the foreground process group for job control.
- Only the focused terminal (global or command widget) should receive/emit signals; no broadcast.
- Bubbleterm should still accept literal ctrl bytes when translation is disabled.

## Design

### PTY termios
- After `pty.Open()`, adjust the slave termios to ensure ISIG is on (and ICANON if we want cooked input). Keep echo as-is.
- Implementation sketch in bubbleterm `emulator.New`:
  - Grab current termios via `unix.IoctlGetTermios(tty.Fd(), unix.TCGETS)` (or BSD `TIOCGETA` as needed).
  - Set `Iflag` |= `unix.ISIG`; optionally set `Lflag` |= `unix.ISIG|unix.ICANON` to fully cooked mode. Keep a copy to restore on Close.
  - Apply with `unix.IoctlSetTermios(..., TCSETS)`.
- Keep raw-mode rendering assumptions untouched; we only tweak signal-related flags.

### Key routing in kc
- Preserve ctrl key data: forward `tea.KeyMsg`/`KeyPressMsg`/`KeyReleaseMsg` directly to bubbleterm (no string rewriting). Already adjusted in `internal/ui/terminal.go`.
- Focus rules: reuse existing routing—when panels are up, only the 2-line terminal gets keys; when a command widget is active, only that widget’s terminal gets keys. Signal translation remains enabled per terminal instance (`WithCtrlCSignal/WithCtrlZSignal(true)`).

### Bubbleterm signal handling
- Keep opt-in translation for ctrl keys; detection should accept both Bubble Tea strings (`"ctrl+c"/"ctrl+z"`) and control-byte forms (`"\x03"/"\x1a"`). Already implemented upstream.
- `SendSignal` should target the process group first (`-pid`), then fall back to the process.
- With ISIG on, most shells/processes receive signals from the kernel automatically; the explicit translation stays as a fallback for raw-like modes or key-protocol variants.

### Edge cases
- Non-shell commands started directly (no job control) still get signals via process group delivery.
- Multiple terminals: each bubbleterm instance has its own focus flag; signals only arise from the instance that receives the key.
- Alt input protocols (kitty/CSI u) still produce ctrl bytes; ensure detection remains byte-friendly.

## Steps to Implement
1) Bubbleterm: re-enable ISIG (and optionally ICANON) on the PTY slave after open; keep original termios for restore.
2) Bubbleterm: ensure signal detection matches ctrl bytes and strings (done), process-group signaling (done).
3) kc: keep direct key forwarding (done) and keep `WithCtrlCSignal/WithCtrlZSignal(true)` on all terminals (done).
4) Verify manually: run `sleep 10` in both the global terminal and command widget; Ctrl+C should terminate; Ctrl+Z should suspend. Add a quick headless test harness if feasible.

## Testing
- Manual: start kc, run `sleep 10` in both terminals, confirm Ctrl+C kills and Ctrl+Z suspends/returns to prompt; resume with `fg`.
- Automated (optional): bubbleheadless script that launches a shell, sends `sleep 1 &`, then `key ctrl+c` and `key ctrl+z`, and inspects job control output.
