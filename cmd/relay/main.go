// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	humanize "github.com/dustin/go-humanize"
	"github.com/micasa-dev/micasa/internal/relay"
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

	encKey, err := parseEncryptionKey(os.Getenv("RELAY_ENCRYPTION_KEY"))
	if err != nil {
		log.Error("startup configuration error", "error", err)
		os.Exit(1)
	}
	store.SetEncryptionKey(encKey)

	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	selfHosted, err := resolveRelayMode(os.Getenv("SELF_HOSTED"), webhookSecret)
	if err != nil {
		log.Error("startup configuration error", "error", err)
		os.Exit(1)
	}

	blobQuota, err := parseBlobQuota(os.Getenv("BLOB_QUOTA"), selfHosted)
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

// parseEncryptionKey decodes a hex-encoded 32-byte AES-256 key.
// Returns an error if the key is missing, malformed, or the wrong length.
func parseEncryptionKey(hexKey string) ([]byte, error) {
	if hexKey == "" {
		return nil, fmt.Errorf(
			"RELAY_ENCRYPTION_KEY is required; generate one with: openssl rand -hex 32",
		)
	}
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("invalid RELAY_ENCRYPTION_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf(
			"RELAY_ENCRYPTION_KEY must be exactly 64 hex characters (32 bytes), got %d chars",
			len(hexKey),
		)
	}
	return key, nil
}

// parseBlobQuota parses the BLOB_QUOTA env var. Accepts human-readable
// sizes (e.g. "5GB", "500MB") via go-humanize, as well as raw byte
// counts. Returns the mode default when envVal is empty.
func parseBlobQuota(envVal string, selfHosted bool) (int64, error) {
	if envVal == "" {
		if selfHosted {
			return 0, nil
		}
		return relay.DefaultBlobQuota, nil
	}
	n, err := humanize.ParseBytes(envVal)
	if err != nil {
		return 0, fmt.Errorf("invalid BLOB_QUOTA %q: %w", envVal, err)
	}
	if n > math.MaxInt64 {
		return 0, fmt.Errorf(
			"BLOB_QUOTA %q exceeds maximum (%d bytes)",
			envVal,
			int64(math.MaxInt64),
		)
	}
	return int64(n), nil
}
