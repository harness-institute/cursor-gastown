//go:build !windows

package tmux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	newSessionSocketDialTimeout  = 200 * time.Millisecond
	newSessionSocketProbeTimeout = time.Second
)

func (t *Tmux) ensureNewSessionSocketSafe() error {
	if t.socketName == "" {
		return nil
	}

	socketPath := filepath.Join(SocketDir(), t.socketName)
	info, err := os.Lstat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("lstat tmux socket %s: %w", socketPath, err)
	}
	mode := info.Mode()
	if mode&os.ModeSymlink != 0 {
		return fmt.Errorf("tmux socket %s is a symlink; refusing to start a new session because tmux could unlink or rebind the target (see gt-h9z)", socketPath)
	}
	if mode&os.ModeSocket == 0 {
		return fmt.Errorf("tmux socket %s is %s, not a Unix socket; refusing to start a new session because tmux could replace it (see gt-h9z)", socketPath, mode.Type())
	}

	return t.ensureLiveSocketSafe(socketPath)
}

func (t *Tmux) ensureLiveSocketSafe(socketPath string) error {
	if stale, err := unixSocketStale(socketPath); err != nil || stale {
		if stale {
			return nil
		}
		return fmt.Errorf("tmux socket %s exists but cannot be safely contacted: %w", socketPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), newSessionSocketProbeTimeout)
	defer cancel()
	if err := t.runListSessionsProbe(ctx); err == nil {
		return nil
	} else if errors.Is(err, ErrNoServer) {
		stale, recheckErr := unixSocketStale(socketPath)
		if recheckErr == nil && stale {
			return nil
		}
		if recheckErr != nil {
			err = fmt.Errorf("%w; socket recheck failed: %v", err, recheckErr)
		}
		return fmt.Errorf("tmux socket %s has a live listener but tmux reported no server; refusing to start a new session because tmux could unlink and rebind it (see gt-h9z): %w", socketPath, err)
	} else {
		return fmt.Errorf("tmux socket %s has a live listener but list-sessions failed; refusing to start a new session because tmux could unlink and rebind it (see gt-h9z): %w", socketPath, err)
	}
}

func unixSocketStale(socketPath string) (bool, error) {
	conn, err := net.DialTimeout("unix", socketPath, newSessionSocketDialTimeout)
	if err != nil {
		if os.IsNotExist(err) || errors.Is(err, syscall.ENOENT) || errors.Is(err, syscall.ECONNREFUSED) {
			return true, nil
		}
		return false, err
	}
	_ = conn.Close()
	return false, nil
}

func (t *Tmux) runListSessionsProbe(ctx context.Context) error {
	stdout, err := os.CreateTemp("", "gt-tmux-probe-stdout-*")
	if err != nil {
		return err
	}
	stdoutPath := stdout.Name()
	defer func() { _ = os.Remove(stdoutPath) }()
	defer func() { _ = stdout.Close() }()

	stderr, err := os.CreateTemp("", "gt-tmux-probe-stderr-*")
	if err != nil {
		return err
	}
	stderrPath := stderr.Name()
	defer func() { _ = os.Remove(stderrPath) }()
	defer func() { _ = stderr.Close() }()

	args := []string{"list-sessions", "-F", ""}
	cmd := t.commandContext(ctx, args...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 100 * time.Millisecond

	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("tmux list-sessions timed out after %s: %w", newSessionSocketProbeTimeout, ctxErr)
		}
		stderrBytes, readErr := os.ReadFile(stderrPath)
		if readErr != nil {
			return fmt.Errorf("tmux list-sessions: %w", err)
		}
		return t.wrapError(err, string(stderrBytes), args)
	}
	return nil
}
