package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	proc "github.com/dylanjbarth/codex-inspector/internal/process"
)

// browserhandoff is a test-only one-shot redirect. It keeps the production
// bootstrap fragment out of terminal output, command arguments, and artifacts.
func main() {
	run := flag.String("run-dir", "", "Inspector run directory")
	route := flag.String("route", "/", "safe dashboard route")
	flag.Parse()
	if *run == "" || !strings.HasPrefix(*route, "/") || strings.Contains(*route, "..") {
		fmt.Fprintln(os.Stderr, "safe run directory and route required")
		os.Exit(2)
	}
	meta, err := proc.Read(*run)
	if err != nil || meta.FragmentToken == "" || meta.FragmentExchanged {
		fmt.Fprintln(os.Stderr, "unexchanged Inspector bootstrap unavailable")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintln(os.Stderr, "handoff unavailable")
		os.Exit(1)
	}
	done := make(chan struct{})
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 2 * time.Second}
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		target := fmt.Sprintf("http://127.0.0.1:%d%s#token=%s&instanceId=%s&protocolVersion=%d", meta.Port, *route, meta.FragmentToken, meta.InstanceID, meta.ProtocolVersion)
		w.Header().Set("Location", target)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusFound)
		select {
		case done <- struct{}{}:
		default:
		}
	})
	fmt.Printf("http://127.0.0.1:%d/\n", listener.Addr().(*net.TCPAddr).Port)
	go func() { _ = server.Serve(listener) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
