// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/cpcloud/micasa/internal/sync"
)

// Handler serves the relay HTTP API.
type Handler struct {
	store Store
	mux   *http.ServeMux
	log   *slog.Logger
}

// NewHandler creates a relay HTTP handler.
func NewHandler(store Store, log *slog.Logger) *Handler {
	h := &Handler{store: store, log: log}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("POST /households", h.handleCreateHousehold)
	mux.HandleFunc("POST /sync/push", h.requireAuth(h.handlePush))
	mux.HandleFunc("GET /sync/pull", h.requireAuth(h.handlePull))
	h.mux = mux
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateHousehold(w http.ResponseWriter, r *http.Request) {
	var req sync.CreateHouseholdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeviceName == "" {
		writeError(w, http.StatusBadRequest, "device_name is required")
		return
	}

	resp, err := h.store.CreateHousehold(r.Context(), req)
	if err != nil {
		h.log.Error("create household", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, resp)
}

type authenticatedHandler func(w http.ResponseWriter, r *http.Request, dev sync.Device)

func (h *Handler) requireAuth(next authenticatedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		dev, err := h.store.AuthenticateDevice(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, r, dev)
	}
}

func (h *Handler) handlePush(w http.ResponseWriter, r *http.Request, dev sync.Device) {
	var req sync.PushRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Ops) == 0 {
		writeError(w, http.StatusBadRequest, "no ops to push")
		return
	}

	// Enforce that all ops belong to the authenticated device's household.
	for i := range req.Ops {
		req.Ops[i].HouseholdID = dev.HouseholdID
		req.Ops[i].DeviceID = dev.ID
	}

	confirmed, err := h.store.Push(r.Context(), req.Ops)
	if err != nil {
		h.log.Error("push", "error", err, "device_id", dev.ID)
		writeError(w, http.StatusInternalServerError, "push failed")
		return
	}
	writeJSON(w, http.StatusOK, sync.PushResponse{Confirmed: confirmed})
}

func (h *Handler) handlePull(w http.ResponseWriter, r *http.Request, dev sync.Device) {
	afterStr := r.URL.Query().Get("after")
	limitStr := r.URL.Query().Get("limit")

	var afterSeq int64
	if afterStr != "" {
		var err error
		afterSeq, err = strconv.ParseInt(afterStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid after parameter")
			return
		}
	}

	limit := 100
	if limitStr != "" {
		n, err := strconv.Atoi(limitStr)
		if err != nil || n < 1 || n > 1000 {
			writeError(w, http.StatusBadRequest, "limit must be 1-1000")
			return
		}
		limit = n
	}

	ops, hasMore, err := h.store.Pull(r.Context(), dev.HouseholdID, dev.ID, afterSeq, limit)
	if err != nil {
		h.log.Error("pull", "error", err, "device_id", dev.ID)
		writeError(w, http.StatusInternalServerError, "pull failed")
		return
	}
	if ops == nil {
		ops = []sync.Envelope{}
	}
	writeJSON(w, http.StatusOK, sync.PullResponse{Ops: ops, HasMore: hasMore})
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
