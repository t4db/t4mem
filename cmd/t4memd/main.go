package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/t4db/t4/pkg/object"
	"github.com/t4db/t4mem/mcp"
	"github.com/t4db/t4mem/memory"
)

func main() {
	args := normalizeCLIArgs(os.Args[1:])
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	rootDir := flags.String("root", defaultRootDir(), "directory for durable memory data")
	socketPath := flags.String("socket", "", "unix socket for daemon mode")
	daemonMode := flags.Bool("daemon", false, "run as the shared memory daemon")
	if err := flags.Parse(args); err != nil {
		log.Fatalf("parse flags: %v", err)
	}

	ctx := context.Background()

	if *socketPath == "" {
		*socketPath = defaultSocketPath(*rootDir)
	}

	if *daemonMode {
		if err := runDaemon(ctx, *rootDir, *socketPath); err != nil {
			log.Fatalf("serve daemon: %v", err)
		}
		return
	}

	if err := runAdapter(ctx, *rootDir, *socketPath); err != nil {
		log.Fatalf("run adapter: %v", err)
	}
}

func runAdapter(ctx context.Context, rootDir, socketPath string) error {
	if err := ensureDaemon(ctx, rootDir, socketPath); err != nil {
		return err
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return fmt.Errorf("dial daemon socket: %w", err)
	}
	defer conn.Close()

	errCh := make(chan error, 2)
	go proxyStream(errCh, conn, os.Stdin)
	go proxyStream(errCh, os.Stdout, conn)

	var firstErr error
	for i := 0; i < 2; i++ {
		err := <-errCh
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
			continue
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runDaemon(ctx context.Context, rootDir, socketPath string) error {
	objectStore, err := loadObjectStoreFromEnv(ctx)
	if err != nil {
		return fmt.Errorf("configure object store: %w", err)
	}

	store, err := memory.Open(memory.Config{RootDir: rootDir, ObjectStore: objectStore})
	if err != nil {
		return fmt.Errorf("open memory store: %w", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("close memory store: %v", err)
		}
	}()

	listener, err := listenUnix(socketPath)
	if err != nil {
		return fmt.Errorf("listen on %q: %w", socketPath, err)
	}
	defer func() {
		_ = listener.Close()
		_ = os.Remove(socketPath)
	}()

	if err := os.Chmod(socketPath, 0o600); err != nil {
		return fmt.Errorf("chmod socket %q: %w", socketPath, err)
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("accept unix socket: %w", err)
		}

		go func() {
			defer conn.Close()
			server := mcp.New(store, conn, conn)
			if err := server.Serve(ctx); err != nil && !errors.Is(err, io.EOF) {
				log.Printf("serve client connection: %v", err)
			}
		}()
	}
}

func defaultSocketPath(rootDir string) string {
	return filepath.Join(rootDir, "daemon.sock")
}

func ensureDaemon(ctx context.Context, rootDir, socketPath string) error {
	_ = ctx
	if conn, err := net.Dial("unix", socketPath); err == nil {
		_ = conn.Close()
		return nil
	}

	if err := startDaemonProcess(rootDir, socketPath); err != nil {
		return err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}

	return fmt.Errorf("daemon did not become ready on %q", socketPath)
}

func startDaemonProcess(rootDir, socketPath string) error {
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return fmt.Errorf("create root dir %q: %w", rootDir, err)
	}

	logPath := filepath.Join(rootDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("open daemon log %q: %w", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.Command(os.Args[0], "-daemon", "-root", rootDir, "-socket", socketPath)
	cmd.Stdin = nil
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start daemon process: %w", err)
	}
	return cmd.Process.Release()
}

func listenUnix(socketPath string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o755); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			if removeErr := os.Remove(socketPath); removeErr != nil {
				return nil, fmt.Errorf("remove non-socket path %q: %w", socketPath, removeErr)
			}
		}
	}

	listener, err := net.Listen("unix", socketPath)
	if err == nil {
		return listener, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) && !strings.Contains(err.Error(), "address already in use") {
		return nil, err
	}

	conn, dialErr := net.DialTimeout("unix", socketPath, 200*time.Millisecond)
	if dialErr == nil {
		_ = conn.Close()
		return nil, fmt.Errorf("socket %q already has an active daemon", socketPath)
	}
	if removeErr := os.Remove(socketPath); removeErr != nil {
		return nil, fmt.Errorf("remove stale socket %q: %w", socketPath, removeErr)
	}
	return net.Listen("unix", socketPath)
}

func proxyStream(errCh chan<- error, dst io.Writer, src io.Reader) {
	_, err := io.Copy(dst, src)
	if closer, ok := dst.(interface{ CloseWrite() error }); ok {
		_ = closer.CloseWrite()
	}
	if conn, ok := dst.(net.Conn); ok {
		_ = conn.Close()
	}
	errCh <- err
}

func defaultRootDir() string {
	if fromEnv := os.Getenv("T4MEM_ROOT"); fromEnv != "" {
		return fromEnv
	}
	return filepath.Join(".", ".t4mem")
}

func loadObjectStoreFromEnv(ctx context.Context) (object.Store, error) {
	bucket := strings.TrimSpace(os.Getenv("T4MEM_S3_BUCKET"))
	if bucket == "" {
		return nil, nil
	}

	cfg := object.S3Config{
		Bucket:          bucket,
		Prefix:          strings.TrimSpace(os.Getenv("T4MEM_S3_PREFIX")),
		Endpoint:        strings.TrimSpace(os.Getenv("T4MEM_S3_ENDPOINT")),
		Region:          strings.TrimSpace(os.Getenv("T4MEM_S3_REGION")),
		Profile:         strings.TrimSpace(os.Getenv("T4MEM_AWS_PROFILE")),
		AccessKeyID:     strings.TrimSpace(os.Getenv("T4MEM_AWS_ACCESS_KEY_ID")),
		SecretAccessKey: strings.TrimSpace(os.Getenv("T4MEM_AWS_SECRET_ACCESS_KEY")),
	}

	if cfg.AccessKeyID == "" && cfg.SecretAccessKey != "" {
		return nil, errors.New("T4MEM_AWS_ACCESS_KEY_ID must be set when T4MEM_AWS_SECRET_ACCESS_KEY is set")
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey == "" {
		return nil, errors.New("T4MEM_AWS_SECRET_ACCESS_KEY must be set when T4MEM_AWS_ACCESS_KEY_ID is set")
	}

	return object.NewS3StoreFromConfig(ctx, cfg)
}

func normalizeCLIArgs(args []string) []string {
	if len(args) != 1 {
		return args
	}

	raw := strings.TrimSpace(args[0])
	if !strings.HasPrefix(raw, "[") {
		return args
	}
	if !strings.HasSuffix(raw, "]") {
		raw += "]"
	}

	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil || len(parsed) == 0 {
		return args
	}
	return parsed
}
