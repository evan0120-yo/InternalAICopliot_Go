// Package main is the entry point for the Internal AI Copilot API server.
// On Cloud Run, HTTP and gRPC are served on the same PORT via h2c (HTTP/2 cleartext).
// Cloud Run terminates TLS externally; the container receives plain HTTP/2.
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"com.citrus.internalaicopilot/internal/app"
	"com.citrus.internalaicopilot/internal/infra"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
)

func main() {
	cfg := infra.LoadConfigFromEnv()
	application, err := app.New(cfg)
	if err != nil {
		log.Fatalf("bootstrap failed: %v", err)
	}
	defer func() {
		if err := application.Close(); err != nil {
			log.Printf("app close failed: %v", err)
		}
	}()

	grpcServer := grpc.NewServer()
	application.RegisterGRPC(grpcServer)

	// Route incoming requests: gRPC → grpcServer, everything else → HTTP handler.
	// h2c allows HTTP/2 over cleartext so Cloud Run can forward gRPC traffic.
	combined := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			application.Handler().ServeHTTP(w, r)
		}
	})

	server := &http.Server{
		Addr:              cfg.Addr,
		Handler:           h2c.NewHandler(combined, &http2.Server{}),
		ReadHeaderTimeout: cfg.ServerReadTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
	}

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-shutdownCtx.Done()
		drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(drainCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
		grpcServer.GracefulStop()
	}()

	log.Printf("Internal AI Copilot listening on %s (HTTP + gRPC via h2c)", cfg.Addr)

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server stopped: %v", err)
	}
}
