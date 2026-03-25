// Package main is the entry point for the Internal AI Copilot API server.
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"com.citrus.internalaicopilot/internal/app"
	"com.citrus.internalaicopilot/internal/infra"

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

	httpServer := &http.Server{
		Addr:              cfg.Addr,
		Handler:           application.Handler(),
		ReadHeaderTimeout: cfg.ServerReadTimeout,
		ReadTimeout:       cfg.ServerReadTimeout,
		WriteTimeout:      cfg.ServerWriteTimeout,
	}
	grpcServer := grpc.NewServer()
	application.RegisterGRPC(grpcServer)

	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		log.Fatalf("gRPC listen failed: %v", err)
	}
	defer grpcListener.Close()

	log.Printf("Internal AI Copilot HTTP listening on %s", cfg.Addr)
	log.Printf("Internal AI Copilot gRPC listening on %s", cfg.GRPCAddr)

	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-shutdownCtx.Done()
		drainCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(drainCtx); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
		stopped := make(chan struct{})
		go func() {
			grpcServer.GracefulStop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-drainCtx.Done():
			grpcServer.Stop()
		}
	}()

	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil {
			if shutdownCtx.Err() != nil {
				return
			}
			log.Printf("gRPC server stopped: %v", err)
			stop()
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("http server stopped: %v", err)
	}
}
