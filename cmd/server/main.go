package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/vovengo/miha-shamanit/internal/config"
	"github.com/vovengo/miha-shamanit/internal/gen"
	"github.com/vovengo/miha-shamanit/internal/httpx"
	"github.com/vovengo/miha-shamanit/internal/service"
)

func main() {
	cfg := config.FromEnv()

	generator := gen.NewFromConfig(cfg)
	app := service.New(cfg, generator)
	handler, err := httpx.NewHandler(cfg, app)
	if err != nil {
		log.Fatalf("build handler: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("Мишаня шаманит слушает %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
