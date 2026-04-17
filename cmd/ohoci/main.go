package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ohoci/internal/app"
)

func main() {
	instance, err := app.New()
	if err != nil {
		log.Fatalf("ohoci startup failed: %v", err)
	}
	defer instance.Close()

	server := &http.Server{
		Addr:              instance.Config.HTTPAddress,
		Handler:           instance.Handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("ohoci listening on %s", instance.Config.HTTPAddress)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("ohoci server failed: %v", err)
	}
}
