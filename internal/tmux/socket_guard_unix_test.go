//go:build !windows

package tmux

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func uniqueSocketName(t *testing.T, prefix string) string {
	t.Helper()
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

func socketPathForTest(t *testing.T, socket string) string {
	t.Helper()
	dir := SocketDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll(%s): %v", dir, err)
	}
	return filepath.Join(dir, socket)
}

func createStaleUnixSocket(t *testing.T, socket string) string {
	t.Helper()
	socketPath := socketPathForTest(t, socket)
	_ = os.Remove(socketPath)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("ListenUnix(%s): %v", socketPath, err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatalf("Close stale listener: %v", err)
	}
	info, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", socketPath, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("%s mode = %s, want Unix socket", socketPath, info.Mode())
	}
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	return socketPath
}

func listenOnSocketPath(t *testing.T, socket string) (*net.UnixListener, string) {
	t.Helper()
	socketPath := socketPathForTest(t, socket)
	_ = os.Remove(socketPath)
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("ListenUnix(%s): %v", socketPath, err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	})
	return listener, socketPath
}

func assertSameSocketPath(t *testing.T, socketPath string, before os.FileInfo) {
	t.Helper()
	after, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatalf("Lstat(%s) after refusal: %v", socketPath, err)
	}
	if after.Mode()&os.ModeSocket == 0 {
		t.Fatalf("%s after refusal mode = %s, want Unix socket", socketPath, after.Mode())
	}
	if !os.SameFile(before, after) {
		t.Fatalf("%s was replaced after refusal", socketPath)
	}
}

func TestEnsureNewSessionSocketSafe(t *testing.T) {
	t.Run("default_socket", func(t *testing.T) {
		if err := (&Tmux{}).ensureNewSessionSocketSafe(); err != nil {
			t.Fatalf("ensureNewSessionSocketSafe(default) = %v", err)
		}
	})

	t.Run("absent_socket", func(t *testing.T) {
		socket := uniqueSocketName(t, "gt-h9z-absent")
		if err := NewTmuxWithSocket(socket).ensureNewSessionSocketSafe(); err != nil {
			t.Fatalf("ensureNewSessionSocketSafe(absent) = %v", err)
		}
	})

	t.Run("stale_unix_socket", func(t *testing.T) {
		socket := uniqueSocketName(t, "gt-h9z-stale")
		createStaleUnixSocket(t, socket)
		if err := NewTmuxWithSocket(socket).ensureNewSessionSocketSafe(); err != nil {
			t.Fatalf("ensureNewSessionSocketSafe(stale) = %v", err)
		}
	})

	t.Run("regular_file", func(t *testing.T) {
		socket := uniqueSocketName(t, "gt-h9z-file")
		socketPath := socketPathForTest(t, socket)
		if err := os.WriteFile(socketPath, []byte("not a socket"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", socketPath, err)
		}
		t.Cleanup(func() { _ = os.Remove(socketPath) })

		err := NewTmuxWithSocket(socket).ensureNewSessionSocketSafe()
		if err == nil {
			t.Fatal("ensureNewSessionSocketSafe(regular file) = nil, want error")
		}
		if !strings.Contains(err.Error(), socketPath) {
			t.Fatalf("error %q does not mention %s", err, socketPath)
		}
	})

	t.Run("directory", func(t *testing.T) {
		socket := uniqueSocketName(t, "gt-h9z-dir")
		socketPath := socketPathForTest(t, socket)
		if err := os.Mkdir(socketPath, 0o700); err != nil {
			t.Fatalf("Mkdir(%s): %v", socketPath, err)
		}
		t.Cleanup(func() { _ = os.Remove(socketPath) })

		if err := NewTmuxWithSocket(socket).ensureNewSessionSocketSafe(); err == nil {
			t.Fatal("ensureNewSessionSocketSafe(directory) = nil, want error")
		}
	})

	t.Run("symlink", func(t *testing.T) {
		socket := uniqueSocketName(t, "gt-h9z-link")
		socketPath := socketPathForTest(t, socket)
		target := socketPath + "-target"
		if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", target, err)
		}
		t.Cleanup(func() { _ = os.Remove(target) })
		if err := os.Symlink(target, socketPath); err != nil {
			t.Skipf("Symlink not supported: %v", err)
		}
		t.Cleanup(func() { _ = os.Remove(socketPath) })

		err := NewTmuxWithSocket(socket).ensureNewSessionSocketSafe()
		if err == nil {
			t.Fatal("ensureNewSessionSocketSafe(symlink) = nil, want error")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error %q should mention symlink", err)
		}
	})

	t.Run("live_tmux_server", func(t *testing.T) {
		if !hasTmux() {
			t.Skip("tmux not installed")
		}
		socket := uniqueSocketName(t, "gt-h9z-live")
		tm := NewTmuxWithSocket(socket)
		t.Cleanup(func() { _ = tm.KillServer() })
		if _, err := tm.run("new-session", "-d", "-s", "gt-h9z-live"); err != nil {
			t.Fatalf("new-session setup: %v", err)
		}
		if err := tm.ensureNewSessionSocketSafe(); err != nil {
			t.Fatalf("ensureNewSessionSocketSafe(live tmux) = %v", err)
		}
	})
}

func TestNewSessionAllowsStaleUnixSocket(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	socket := uniqueSocketName(t, "gt-h9z-newsession-stale")
	createStaleUnixSocket(t, socket)
	tm := NewTmuxWithSocket(socket)
	t.Cleanup(func() { _ = tm.KillServer() })

	if err := tm.NewSession("gt-h9z-stale-ok", ""); err != nil {
		t.Fatalf("NewSession against stale Unix socket = %v, want success", err)
	}
}

func TestNewSessionRefusesUnresponsiveSocket(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	socket := uniqueSocketName(t, "gt-h9z-unresponsive")
	listener, socketPath := listenOnSocketPath(t, socket)
	var heldMu sync.Mutex
	var held []*net.UnixConn
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			heldMu.Lock()
			held = append(held, conn)
			heldMu.Unlock()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
		heldMu.Lock()
		defer heldMu.Unlock()
		for _, conn := range held {
			_ = conn.Close()
		}
	})

	before, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", socketPath, err)
	}
	errCh := make(chan error, 1)
	start := time.Now()
	go func() {
		errCh <- NewTmuxWithSocket(socket).NewSession("gt-h9z-unresponsive", "")
	}()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("NewSession against unresponsive listener = nil, want error")
		}
		if elapsed := time.Since(start); elapsed > 3*time.Second {
			t.Fatalf("NewSession took %s, want bounded refusal", elapsed)
		}
	case <-time.After(4 * time.Second):
		_ = listener.Close()
		t.Fatal("NewSession against unresponsive listener hung")
	}
	assertSameSocketPath(t, socketPath, before)
}

func TestNewSessionRefusesClosingListener(t *testing.T) {
	if !hasTmux() {
		t.Skip("tmux not installed")
	}
	socket := uniqueSocketName(t, "gt-h9z-closing")
	listener, socketPath := listenOnSocketPath(t, socket)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.AcceptUnix()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-done
	})

	before, err := os.Lstat(socketPath)
	if err != nil {
		t.Fatalf("Lstat(%s): %v", socketPath, err)
	}
	if err := NewTmuxWithSocket(socket).NewSession("gt-h9z-closing", ""); err == nil {
		t.Fatal("NewSession against closing listener = nil, want error")
	}
	assertSameSocketPath(t, socketPath, before)
}

func TestNewSessionVariantsUseSocketGuard(t *testing.T) {
	socket := uniqueSocketName(t, "gt-h9z-variants")
	socketPath := socketPathForTest(t, socket)
	if err := os.WriteFile(socketPath, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", socketPath, err)
	}
	t.Cleanup(func() { _ = os.Remove(socketPath) })
	tm := NewTmuxWithSocket(socket)

	tests := []struct {
		name string
		run  func(string) error
	}{
		{name: "NewSession", run: func(name string) error { return tm.NewSession(name, "") }},
		{name: "NewSessionWithCommand", run: func(name string) error { return tm.NewSessionWithCommand(name, "", "sleep 5") }},
		{name: "NewSessionWithCommandAndEnv", run: func(name string) error {
			return tm.NewSessionWithCommandAndEnv(name, "", "sleep 5", map[string]string{"GT_TEST": "1"})
		}},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.run(fmt.Sprintf("gt-h9z-variant-%d", i))
			if err == nil {
				t.Fatal("creation variant returned nil, want socket guard error")
			}
			if !strings.Contains(err.Error(), socketPath) {
				t.Fatalf("error %q does not mention %s", err, socketPath)
			}
		})
	}
}
