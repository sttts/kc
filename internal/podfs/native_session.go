package podfs

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

const (
	cmdPrefixReady    = "__kc_ready__"
	cmdPrefixLSDone   = "__kc_ls_done__"
	cmdPrefixLSRow    = "__kc_ls_row__"
	cmdPrefixCatDone  = "__kc_cat_done__"
	cmdPrefixCatMeta  = "__kc_cat_meta__"
	cmdPrefixCatChunk = "__kc_cat_chunk__"
	cmdPrefixError    = "__kc_err__"
)

type nativeSession struct {
	spec    SessionSpec
	stdin   io.WriteCloser
	lineCh  chan string
	doneCh  chan struct{}
	errOnce sync.Once
	err     error
	cancel  context.CancelFunc
	mu      sync.Mutex
	closed  bool
}

func newNativeSession(ctx context.Context, cfg *rest.Config, client kubernetes.Interface, spec SessionSpec) (ExecSession, error) {
	ctx, cancel := context.WithCancel(ctx)
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()
	stderrReader, stderrWriter := io.Pipe()

	req := client.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(spec.Pod).
		Namespace(spec.Namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: spec.Container,
			Command:   []string{"sh"},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		cancel()
		_ = stdinReader.Close()
		_ = stdinWriter.Close()
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		_ = stderrReader.Close()
		_ = stderrWriter.Close()
		return nil, fmt.Errorf("podfs: executor: %w", err)
	}

	session := &nativeSession{
		spec:   spec,
		stdin:  stdinWriter,
		lineCh: make(chan string, 256),
		doneCh: make(chan struct{}),
		cancel: cancel,
	}

	go session.copyLoop(exec, ctx, stdinReader, stdoutWriter, stderrWriter)
	go session.stderrDrain(stderrReader)
	go session.stdoutLoop(stdoutReader)

	if err := session.bootstrap(ctx); err != nil {
		session.Close()
		return nil, err
	}
	return session, nil
}

func (s *nativeSession) copyLoop(exec remotecommand.Executor, ctx context.Context, stdin io.ReadCloser, stdout, stderr io.WriteCloser) {
	err := exec.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdin:  stdin,
		Stdout: stdout,
		Stderr: stderr,
		Tty:    false,
	})
	_ = stdout.Close()
	_ = stderr.Close()
	_ = stdin.Close()
	s.setError(err)
	close(s.doneCh)
}

func (s *nativeSession) stdoutLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		s.lineCh <- line
	}
	if err := scanner.Err(); err != nil {
		s.setError(err)
	}
	close(s.lineCh)
}

func (s *nativeSession) stderrDrain(r io.Reader) {
	// Drain stderr so the remote exec can't block. We don't currently surface stderr.
	buf := bufio.NewScanner(r)
	for buf.Scan() {
		// TODO: feed to logger if needed
	}
}

func (s *nativeSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.cancel()
	_ = s.stdin.Close()
	return nil
}

func (s *nativeSession) bootstrap(ctx context.Context) error {
	if err := s.writeScript(bootstrapScript); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case line, ok := <-s.lineCh:
			if !ok {
				return s.sessionError()
			}
			switch {
			case strings.HasPrefix(line, cmdPrefixReady):
				return nil
			case strings.HasPrefix(line, cmdPrefixError):
				return fmt.Errorf("podfs bootstrap error: %s", strings.TrimPrefix(line, cmdPrefixError+"|"))
			default:
				continue
			}
		}
	}
}

func (s *nativeSession) List(ctx context.Context, path string) ([]FileEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeCommand("__kc_ls " + quoteArg(path)); err != nil {
		return nil, err
	}
	entries := make([]FileEntry, 0)
	for {
		line, err := s.nextLine(ctx)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(line, cmdPrefixError) {
			return nil, fmt.Errorf("podfs ls error: %s", strings.TrimPrefix(line, cmdPrefixError+"|"))
		}
		if strings.HasPrefix(line, cmdPrefixLSDone) {
			return entries, nil
		}
		if !strings.HasPrefix(line, cmdPrefixLSRow) {
			continue
		}
		row, err := decodeEntry(strings.TrimPrefix(line, cmdPrefixLSRow))
		if err != nil {
			return nil, err
		}
		entries = append(entries, row)
	}
}

func (s *nativeSession) ReadFile(ctx context.Context, path string, limit int64) (io.ReadCloser, error) {
	s.mu.Lock()
	cmd := fmt.Sprintf("__kc_cat %s %d", quoteArg(path), limit)
	if err := s.writeCommand(cmd); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	var expected int64
	for {
		line, err := s.nextLine(ctx)
		if err != nil {
			s.mu.Unlock()
			return nil, err
		}
		switch {
		case strings.HasPrefix(line, cmdPrefixError):
			s.mu.Unlock()
			return nil, fmt.Errorf("podfs cat error: %s", strings.TrimPrefix(line, cmdPrefixError+"|"))
		case strings.HasPrefix(line, cmdPrefixCatMeta):
			val := strings.TrimPrefix(line, cmdPrefixCatMeta+"|")
			expected, err = strconv.ParseInt(val, 10, 64)
			if err != nil {
				s.mu.Unlock()
				return nil, fmt.Errorf("podfs: parse cat meta: %w", err)
			}
			if expected == 0 {
				// Still need to consume done marker
				if err := s.consumeCatDone(ctx); err != nil {
					s.mu.Unlock()
					return nil, err
				}
				s.mu.Unlock()
				return io.NopCloser(strings.NewReader("")), nil
			}
			reader := newChunkReader(s, ctx, expected, func() {
				s.mu.Unlock()
			})
			return reader, nil
		default:
			continue
		}
	}
}

func (s *nativeSession) consumeCatDone(ctx context.Context) error {
	for {
		line, err := s.nextLine(ctx)
		if err != nil {
			return err
		}
		if strings.HasPrefix(line, cmdPrefixCatDone) {
			return nil
		}
		if strings.HasPrefix(line, cmdPrefixError) {
			return fmt.Errorf("podfs cat error: %s", strings.TrimPrefix(line, cmdPrefixError+"|"))
		}
	}
}

func (s *nativeSession) nextLine(ctx context.Context) (string, error) {
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case line, ok := <-s.lineCh:
			if !ok {
				return "", s.sessionError()
			}
			return line, nil
		case <-s.doneCh:
			return "", s.sessionError()
		}
	}
}

func (s *nativeSession) setError(err error) {
	if err == nil {
		return
	}
	s.errOnce.Do(func() {
		s.err = err
	})
}

func (s *nativeSession) sessionError() error {
	if s.err != nil {
		return s.err
	}
	return io.EOF
}

func (s *nativeSession) writeScript(script string) error {
	_, err := io.WriteString(s.stdin, script+"\n")
	return err
}

func (s *nativeSession) writeCommand(cmd string) error {
	_, err := io.WriteString(s.stdin, cmd+"\n")
	return err
}

func quoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(arg, "'", `'"'"'`) + "'"
}

func decodeEntry(payload string) (FileEntry, error) {
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return FileEntry{}, fmt.Errorf("podfs: decode entry: %w", err)
	}
	parts := strings.Split(string(data), "|")
	if len(parts) < 6 {
		return FileEntry{}, fmt.Errorf("podfs: malformed entry %q", string(data))
	}
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return FileEntry{}, err
	}
	modUnix, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return FileEntry{}, err
	}
	mode, err := strconv.ParseUint(parts[0], 16, 32)
	if err != nil {
		return FileEntry{}, err
	}
	entryType := EntryType(parts[3])
	if entryType == "" {
		entryType = EntryTypeOther
	}
	return FileEntry{
		Name:      parts[4],
		Path:      parts[4],
		Type:      entryType,
		Size:      size,
		Mode:      uint32(mode),
		UpdatedAt: time.Unix(modUnix, 0),
		Target:    parts[5],
	}, nil
}

type chunkReader struct {
	session     *nativeSession
	ctx         context.Context
	expected    int64
	read        int64
	buf         []byte
	closed      bool
	releaseOnce sync.Once
	release     func()
}

func newChunkReader(s *nativeSession, ctx context.Context, expected int64, release func()) *chunkReader {
	return &chunkReader{
		session:  s,
		ctx:      ctx,
		expected: expected,
		buf:      make([]byte, 0, 8192),
		release:  release,
	}
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.closed {
		return 0, io.EOF
	}
	for len(r.buf) == 0 && r.read < r.expected {
		line, err := r.session.nextLine(r.ctx)
		if err != nil {
			r.finish()
			return 0, err
		}
		switch {
		case strings.HasPrefix(line, cmdPrefixCatChunk):
			payload := strings.TrimPrefix(line, cmdPrefixCatChunk)
			data, err := base64.StdEncoding.DecodeString(payload)
			if err != nil {
				r.finish()
				return 0, err
			}
			r.buf = append(r.buf, data...)
		case strings.HasPrefix(line, cmdPrefixCatDone):
			r.finish()
			if r.read < r.expected && len(r.buf) == 0 {
				return 0, io.EOF
			}
		case strings.HasPrefix(line, cmdPrefixError):
			r.finish()
			return 0, fmt.Errorf("podfs cat error: %s", strings.TrimPrefix(line, cmdPrefixError+"|"))
		case strings.HasPrefix(line, cmdPrefixCatMeta):
			// already received meta; ignore duplicates
			continue
		default:
			continue
		}
	}
	if len(r.buf) == 0 {
		r.finish()
		return 0, io.EOF
	}
	n := copy(p, r.buf)
	r.buf = r.buf[n:]
	r.read += int64(n)
	if r.read >= r.expected {
		// Drain until done marker
		_ = r.session.consumeCatDone(r.ctx)
		r.finish()
		return n, io.EOF
	}
	return n, nil
}

func (r *chunkReader) Close() error {
	r.finish()
	return nil
}

func (r *chunkReader) finish() {
	if r.closed {
		return
	}
	r.closed = true
	if r.release != nil {
		r.releaseOnce.Do(r.release)
	}
}

const bootstrapScript = `
__kc_prereq() {
  for cmd in stat base64 dd; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
      printf '__kc_err__|bootstrap|missing:%s\n' "$cmd"
      return 1
    fi
  done
  return 0
}
if ! __kc_prereq; then
  exit 1
fi

__kc_encode_row() {
  printf '%s' "$1" | base64 | tr -d '\n'
}

__kc_emit_cat_chunks() {
  while IFS= read -r chunk; do
    printf '__kc_cat_chunk__%s\n' "$chunk"
  done
}

__kc_entry_type() {
  path="$1"
  if [ -d "$path" ]; then
    printf 'dir'
  elif [ -h "$path" ]; then
    printf 'symlink'
  elif [ -p "$path" ]; then
    printf 'pipe'
  elif [ -S "$path" ]; then
    printf 'socket'
  elif [ -b "$path" ] || [ -c "$path" ]; then
    printf 'device'
  else
    printf 'file'
  fi
}

__kc_ls() {
  dir="$1"
  if ! cd -- "$dir" 2>/dev/null; then
    printf '__kc_err__|ls|ENOENT %s\n' "$dir"
    printf '__kc_ls_done__\n'
    return
  fi
  for f in .* *; do
    [ "$f" = "." ] && continue
    [ "$f" = ".." ] && continue
    [ -e "$f" ] || continue
    mode=$(stat -c '%f' -- "$f" 2>/dev/null || printf '0')
    size=$(stat -c '%s' -- "$f" 2>/dev/null || printf '0')
    mtime=$(stat -c '%Y' -- "$f" 2>/dev/null || printf '0')
    etype=$(__kc_entry_type "$f")
    target=''
    if [ "$etype" = "symlink" ]; then
      target=$(readlink -- "$f" 2>/dev/null || printf '')
    fi
    row=$(printf '%s|%s|%s|%s|%s|%s' "$mode" "$size" "$mtime" "$etype" "$f" "$target")
    printf '__kc_ls_row__%s\n' "$(__kc_encode_row "$row")"
  done
  printf '__kc_ls_done__\n'
}

__kc_cat() {
  path="$1"
  limit="$2"
  if [ ! -r "$path" ]; then
    printf '__kc_err__|cat|EACCES %s\n' "$path"
    printf '__kc_cat_done__\n'
    return
  fi
  total=$(stat -c '%s' -- "$path" 2>/dev/null || printf '0')
  if [ "$limit" -gt 0 ] && [ "$limit" -lt "$total" ]; then
    total="$limit"
  fi
  printf '__kc_cat_meta__|%s\n' "$total"
  if [ "$total" -eq 0 ]; then
    printf '__kc_cat_done__\n'
    return
  fi
  if [ "$limit" -gt 0 ]; then
    dd if="$path" bs=4096 count=$(( (limit + 4095) / 4096 )) 2>/dev/null | base64 | __kc_emit_cat_chunks
  else
    base64 <"$path" | __kc_emit_cat_chunks
  fi
  printf '__kc_cat_done__\n'
}

printf '__kc_ready__\n'
`
