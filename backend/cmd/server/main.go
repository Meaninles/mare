package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mam/backend/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx)
	if err != nil {
		slog.Error("backend bootstrap failed", "error", err)
		os.Exit(1)
	}

	if err := application.Run(ctx); err != nil {
		slog.Error("backend stopped with error", "error", err)
		os.Exit(1)
	}
}
