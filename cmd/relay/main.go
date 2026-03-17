// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cpcloud/micasa/internal/relay"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	var store relay.Store
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL != "" {
		pg, err := relay.OpenPgStore(databaseURL)
		if err != nil {
			log.Error("open postgres store", "error", err)
			os.Exit(1)
		}
		if err := pg.AutoMigrate(); err != nil {
			log.Error("auto migrate", "error", err)
			os.Exit(1)
		}
		store = pg
		log.Info("using postgres store")
	} else {
		store = relay.NewMemStore()
		log.Info("using in-memory store (no DATABASE_URL set)")
	}

	var handlerOpts []relay.HandlerOption
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	if webhookSecret != "" {
		handlerOpts = append(handlerOpts, relay.WithWebhookSecret(webhookSecret))
		log.Info("Stripe webhook verification enabled")
	} else {
		log.Warn("STRIPE_WEBHOOK_SECRET not set -- Stripe webhooks will be rejected")
	}

	handler := relay.NewHandler(store, log, handlerOpts...)

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("relay server starting", "port", port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	log.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown error", "error", err)
	}
	if err := store.Close(); err != nil {
		log.Error("store close error", "error", err)
	}
}
