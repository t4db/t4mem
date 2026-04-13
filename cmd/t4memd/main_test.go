package main

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeCLIArgsExpandsSerializedArray(t *testing.T) {
	t.Parallel()

	got := normalizeCLIArgs([]string{`["-root", "/tmp/t4mem"`})
	want := []string{"-root", "/tmp/t4mem"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCLIArgs() = %#v, want %#v", got, want)
	}
}

func TestNormalizeCLIArgsLeavesRegularArgsAlone(t *testing.T) {
	t.Parallel()

	got := normalizeCLIArgs([]string{"-root", "/tmp/t4mem"})
	want := []string{"-root", "/tmp/t4mem"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeCLIArgs() = %#v, want %#v", got, want)
	}
}

func TestDefaultSocketPathUsesRootDir(t *testing.T) {
	t.Parallel()

	root := filepath.Join("/tmp", "t4mem-root")
	got := defaultSocketPath(root)
	want := filepath.Join(root, "daemon.sock")
	if got != want {
		t.Fatalf("defaultSocketPath() = %q, want %q", got, want)
	}
}

func TestListenUnixRemovesStaleSocket(t *testing.T) {
	t.Parallel()

	dir, err := os.MkdirTemp("/tmp", "t4memd-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	defer os.RemoveAll(dir)

	socketPath := filepath.Join(dir, "daemon.sock")
	if err := os.WriteFile(socketPath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale socket placeholder: %v", err)
	}

	listener, err := listenUnix(socketPath)
	if err != nil {
		t.Fatalf("listenUnix() error = %v", err)
	}
	defer listener.Close()

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial unix socket: %v", err)
	}
	_ = conn.Close()
}
