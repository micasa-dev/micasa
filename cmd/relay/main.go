// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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
			_ = pg.Close()
			os.Exit(1)
		}
		store = pg
		log.Info("using postgres store")
	} else {
		store = relay.NewMemStore()
		log.Info("using in-memory store (no DATABASE_URL set)")
	}

	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	selfHosted, err := resolveRelayMode(os.Getenv("SELF_HOSTED"), webhookSecret)
	if err != nil {
		log.Error("startup configuration error", "error", err)
		os.Exit(1)
	}

	blobQuota, err := parseBlobQuota(os.Getenv("BLOB_QUOTA_BYTES"), selfHosted)
	if err != nil {
		log.Error("startup configuration error", "error", err)
		os.Exit(1)
	}

	var handlerOpts []relay.HandlerOption
	if selfHosted {
		handlerOpts = append(handlerOpts, relay.WithSelfHosted())
		log.Info("running in self-hosted mode")
	}
	handlerOpts = append(handlerOpts, relay.WithBlobQuota(blobQuota))

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

// resolveRelayMode determines whether the relay runs in self-hosted mode.
// Returns an error if SELF_HOSTED and STRIPE_WEBHOOK_SECRET are both set.
func resolveRelayMode(selfHostedEnv, webhookSecret string) (bool, error) {
	var selfHosted bool
	if selfHostedEnv != "" {
		var err error
		selfHosted, err = strconv.ParseBool(selfHostedEnv)
		if err != nil {
			return false, fmt.Errorf(
				"invalid SELF_HOSTED value %q: must be a boolean (true/false/1/0)", selfHostedEnv,
			)
		}
	}
	if selfHosted && webhookSecret != "" {
		return false, fmt.Errorf(
			"SELF_HOSTED=true and STRIPE_WEBHOOK_SECRET are mutually exclusive -- " +
				"set one or the other, not both",
		)
	}
	return selfHosted, nil
}

// parseBlobQuota parses the BLOB_QUOTA_BYTES env var. Returns the mode
// default when envVal is empty. Returns an error for negative or
// non-integer values.
func parseBlobQuota(envVal string, selfHosted bool) (int64, error) {
	if envVal == "" {
		if selfHosted {
			return 0, nil
		}
		return relay.DefaultBlobQuota, nil
	}
	n, err := strconv.ParseInt(envVal, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid BLOB_QUOTA_BYTES %q: %w", envVal, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("BLOB_QUOTA_BYTES must be non-negative, got %d", n)
	}
	return n, nil
}
