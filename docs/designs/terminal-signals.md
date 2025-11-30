# Terminal Signals Design

## Goal
Make ctrl-based signals (SIGINT/SIGTSTP) behave like a normal TTY for all embedded terminals: the global 2-line terminal and command widgets. Ctrl+C/Z should hit the foreground process group (shell + child), not just the shell line editor.

## Constraints & Observations
- Bubbleterm now re-enables ISIG (and ICANON) on the PTY slave after open, so the kernel synthesizes SIGINT/SIGTSTP from `^C/^Z` like a normal TTY.
- `SendSignal` still exists for programmatic delivery, but the ctrl-key toggles were removed; signals should flow from the kernel, not from manual translation.
- Kc wraps Bubble Tea key messages; we must pass ctrl keys through untouched so the TTY sees the control bytes.

## Desired Behavior
- Ctrl+C: deliver SIGINT to the foreground process group of the PTY (shell and its child). Should stop `sleep 10` etc.
- Ctrl+Z: deliver SIGTSTP to the foreground process group for job control.
- Only the focused terminal (global or command widget) should receive/emit signals; no broadcast.
- Bubbleterm should still accept literal ctrl bytes; no app-level translation required.

## Design

### PTY termios
- After `pty.Open()`, adjust the slave termios to ensure ISIG (and ICANON) are on; keep echo as-is and restore on Close.
- Platform helpers select the right ioctl (`TCGETS/TCSETS` on Linux, `TIOCGETA/TIOCSETA` on BSD/Darwin).
- Rendering stays unchanged; only signal-related flags are tweaked.

### Key routing in kc
- Preserve ctrl key data: forward `tea.KeyMsg`/`KeyPressMsg`/`KeyReleaseMsg` directly to bubbleterm (no string rewriting).
- Focus rules: reuse existing routing—when panels are up, only the 2-line terminal gets keys; when a command widget is active, only that widget’s terminal gets keys. No ctrl-key translation is needed in kc or bubbleterm.

### Bubbleterm signal handling
- Rely on kernel-delivered signals via ISIG/ICANON; no ctrl-key translation path is used.
- `SendSignal` remains for programmatic uses and targets the process group first (`-pid`), then falls back to the process.

### Edge cases
- Non-shell commands started directly (no job control) still get signals via process group delivery.
- Multiple terminals: each bubbleterm instance has its own focus flag; signals only arise from the instance that receives the key.
- Alt input protocols (kitty/CSI u) still produce ctrl bytes; ensure detection remains byte-friendly.

## Steps to Implement
1) Bubbleterm: enable ISIG/ICANON on the PTY slave after open and restore termios on close. ✅
2) Bubbleterm: keep `SendSignal` for direct calls, but remove ctrl-key toggles. ✅
3) kc: forward key msgs untouched; no ctrl-key toggles needed. ✅
4) Verify manually: run `sleep 10` in both the global terminal and command widget; Ctrl+C should terminate; Ctrl+Z should suspend. Headless test harness optional.

## Testing
- Manual: start kc, run `sleep 10` in both terminals, confirm Ctrl+C kills and Ctrl+Z suspends/returns to prompt; resume with `fg`.
- Automated (optional): bubbleheadless script that launches a shell, sends `sleep 1 &`, then `key ctrl+c` and `key ctrl+z`, and inspects job control output.
