package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"pr-reviewer/internal/api"
	"pr-reviewer/internal/handlers"
	"pr-reviewer/internal/migrations"
	"pr-reviewer/internal/repo"
	"pr-reviewer/internal/service"
	"pr-reviewer/pkg/config"
)

func main() {
	cfg := config.FromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := repo.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db connect error: %v", err)
	}
	defer pool.Close()

	if err := migrations.Run(ctx, pool); err != nil {
		log.Fatalf("db migrate error: %v", err)
	}
	log.Println("migrations applied")

	repository := repo.NewRepo(pool)
	svc := service.NewService(repository)
	apiServer := handlers.NewServer(svc)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      api.Handler(apiServer),
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	go func() {
		log.Printf("listening on :%s\n", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}
