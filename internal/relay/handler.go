// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/cpcloud/micasa/internal/sync"
)

const maxRequestBody = 1 << 20 // 1 MB

// Handler serves the relay HTTP API.
type Handler struct {
	store         Store
	mux           *http.ServeMux
	log           *slog.Logger
	webhookSecret string
}

// NewHandler creates a relay HTTP handler. The webhookSecret is used to
// verify Stripe webhook signatures; pass empty string to disable webhook
// verification (useful for testing).
func NewHandler(store Store, log *slog.Logger, opts ...HandlerOption) *Handler {
	h := &Handler{store: store, log: log}
	for _, opt := range opts {
		opt(h)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.handleHealth)
	mux.HandleFunc("POST /households", h.handleCreateHousehold)
	mux.HandleFunc("POST /sync/push", h.requireAuth(h.requireSubscription(h.handlePush)))
	mux.HandleFunc("GET /sync/pull", h.requireAuth(h.requireSubscription(h.handlePull)))
	mux.HandleFunc("POST /households/{id}/invite", h.requireAuth(h.handleCreateInvite))
	mux.HandleFunc("POST /households/{id}/join", h.handleJoin)
	mux.HandleFunc(
		"GET /households/{id}/pending-exchanges",
		h.requireAuth(h.handleGetPendingExchanges),
	)
	mux.HandleFunc("POST /key-exchange/{id}/complete", h.requireAuth(h.handleCompleteKeyExchange))
	mux.HandleFunc("GET /key-exchange/{id}", h.handleGetKeyExchangeResult)
	mux.HandleFunc("GET /households/{id}/devices", h.requireAuth(h.handleListDevices))
	mux.HandleFunc(
		"DELETE /households/{id}/devices/{device_id}",
		h.requireAuth(h.handleRevokeDevice),
	)
	mux.HandleFunc(
		"PUT /blobs/{household_id}/{hash}",
		h.requireAuth(h.requireSubscription(h.handlePutBlob)),
	)
	mux.HandleFunc(
		"GET /blobs/{household_id}/{hash}",
		h.requireAuth(h.requireSubscription(h.handleGetBlob)),
	)
	mux.HandleFunc(
		"HEAD /blobs/{household_id}/{hash}",
		h.requireAuth(h.requireSubscription(h.handleHeadBlob)),
	)
	mux.HandleFunc("GET /status", h.requireAuth(h.handleStatus))
	mux.HandleFunc("POST /webhooks/stripe", h.handleStripeWebhook)
	h.mux = mux
	return h
}

// HandlerOption configures the relay handler.
type HandlerOption func(*Handler)

// WithWebhookSecret sets the Stripe webhook signing secret.
func WithWebhookSecret(secret string) HandlerOption {
	return func(h *Handler) { h.webhookSecret = secret }
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *Handler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleCreateHousehold(w http.ResponseWriter, r *http.Request) {
	var req sync.CreateHouseholdRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DeviceName == "" {
		writeError(w, http.StatusBadRequest, "device_name is required")
		return
	}
	if len(req.PublicKey) == 0 {
		writeError(w, http.StatusBadRequest, "public_key is required")
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

// requireSubscription wraps an authenticated handler and checks that the
// device's household has an active Stripe subscription. Returns 402 if
// the subscription is explicitly non-active. Empty status (no subscription
// configured) is allowed (dev/free mode).
func (h *Handler) requireSubscription(next authenticatedHandler) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, dev sync.Device) {
		hh, err := h.store.GetHousehold(r.Context(), dev.HouseholdID)
		if err != nil {
			h.log.Error("get household for subscription check", "error", err)
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		if hh.StripeStatus != "" && hh.StripeStatus != sync.SubscriptionActive {
			writeError(w, http.StatusPaymentRequired, "subscription inactive")
			return
		}
		next(w, r, dev)
	}
}

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
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(&req); err != nil {
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

func (h *Handler) handleCreateInvite(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("id")
	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}

	invite, err := h.store.CreateInvite(r.Context(), hhID, dev.ID)
	if err != nil {
		h.log.Error("create invite", "error", err)
		writeError(w, http.StatusBadRequest, "failed to create invite")
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (h *Handler) handleJoin(w http.ResponseWriter, r *http.Request) {
	var req sync.JoinRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.InviteCode == "" {
		writeError(w, http.StatusBadRequest, "invite_code is required")
		return
	}
	if req.DeviceName == "" {
		writeError(w, http.StatusBadRequest, "device_name is required")
		return
	}
	if len(req.PublicKey) == 0 {
		writeError(w, http.StatusBadRequest, "public_key is required")
		return
	}

	hhID := r.PathValue("id")

	resp, err := h.store.StartJoin(r.Context(), hhID, req.InviteCode, req)
	if err != nil {
		h.log.Error("start join", "error", err)
		writeError(w, http.StatusBadRequest, "invalid or expired invite code")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleGetPendingExchanges(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("id")
	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}

	exchanges, err := h.store.GetPendingExchanges(r.Context(), hhID)
	if err != nil {
		h.log.Error("get pending exchanges", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if exchanges == nil {
		exchanges = []sync.PendingKeyExchange{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"exchanges": exchanges})
}

func (h *Handler) handleCompleteKeyExchange(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	exchangeID := r.PathValue("id")

	var req sync.CompleteKeyExchangeRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.EncryptedHouseholdKey) == 0 {
		writeError(w, http.StatusBadRequest, "encrypted_household_key is required")
		return
	}

	err := h.store.CompleteKeyExchange(
		r.Context(),
		dev.HouseholdID,
		exchangeID,
		req.EncryptedHouseholdKey,
	)
	if err != nil {
		h.log.Error("complete key exchange", "error", err)
		writeError(w, http.StatusBadRequest, "key exchange failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handleGetKeyExchangeResult(w http.ResponseWriter, r *http.Request) {
	exchangeID := r.PathValue("id")

	result, err := h.store.GetKeyExchangeResult(r.Context(), exchangeID)
	if err != nil {
		h.log.Error("get key exchange result", "error", err)
		writeError(w, http.StatusNotFound, "key exchange not found")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleListDevices(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("id")
	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}

	devices, err := h.store.ListDevices(r.Context(), hhID)
	if err != nil {
		h.log.Error("list devices", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if devices == nil {
		devices = []sync.Device{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (h *Handler) handleRevokeDevice(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("id")
	deviceID := r.PathValue("device_id")

	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}
	if deviceID == dev.ID {
		writeError(w, http.StatusBadRequest, "cannot revoke your own device")
		return
	}

	err := h.store.RevokeDevice(r.Context(), hhID, deviceID)
	if err != nil {
		h.log.Error("revoke device", "error", err)
		writeError(w, http.StatusBadRequest, "device revocation failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) handlePutBlob(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("household_id")
	hash := r.PathValue("hash")

	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}
	if !validSHA256Hash(hash) {
		writeError(w, http.StatusBadRequest, "invalid hash: must be 64 lowercase hex characters")
		return
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, maxBlobSize+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}
	if int64(len(data)) > maxBlobSize {
		writeError(w, http.StatusRequestEntityTooLarge, "blob exceeds maximum size (50 MB)")
		return
	}

	if err := h.store.PutBlob(r.Context(), hhID, hash, data); err != nil {
		switch {
		case errors.Is(err, errBlobExists):
			writeJSON(w, http.StatusConflict, map[string]string{"status": "exists"})
		case errors.Is(err, errQuotaExceeded):
			usage, _ := h.store.BlobUsage(r.Context(), hhID)
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]any{
				"error":       "blob storage quota exceeded",
				"used_bytes":  usage,
				"quota_bytes": defaultBlobQuota,
			})
		default:
			h.log.Error("put blob", "error", err)
			writeError(w, http.StatusInternalServerError, "store blob failed")
		}
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"status": "ok"})
}

func (h *Handler) handleGetBlob(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("household_id")
	hash := r.PathValue("hash")

	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}

	data, err := h.store.GetBlob(r.Context(), hhID, hash)
	if err != nil {
		if errors.Is(err, errBlobNotFound) {
			writeError(w, http.StatusNotFound, "blob not found")
			return
		}
		h.log.Error("get blob", "error", err)
		writeError(w, http.StatusInternalServerError, "get blob failed")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) handleHeadBlob(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	hhID := r.PathValue("household_id")
	hash := r.PathValue("hash")

	if dev.HouseholdID != hhID {
		writeError(w, http.StatusForbidden, "device does not belong to this household")
		return
	}

	exists, err := h.store.HasBlob(r.Context(), hhID, hash)
	if err != nil {
		h.log.Error("head blob", "error", err)
		writeError(w, http.StatusInternalServerError, "check blob failed")
		return
	}
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) handleStatus(
	w http.ResponseWriter,
	r *http.Request,
	dev sync.Device,
) {
	devices, err := h.store.ListDevices(r.Context(), dev.HouseholdID)
	if err != nil {
		h.log.Error("status: list devices", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	hh, err := h.store.GetHousehold(r.Context(), dev.HouseholdID)
	if err != nil {
		h.log.Error("status: get household", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	opsCount, err := h.store.OpsCount(r.Context(), dev.HouseholdID)
	if err != nil {
		h.log.Error("status: ops count", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	blobUsed, err := h.store.BlobUsage(r.Context(), dev.HouseholdID)
	if err != nil {
		h.log.Error("status: blob usage", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, sync.StatusResponse{
		HouseholdID:  dev.HouseholdID,
		Devices:      devices,
		OpsCount:     opsCount,
		StripeStatus: hh.StripeStatus,
		BlobStorage: &sync.BlobStorage{
			UsedBytes:  blobUsed,
			QuotaBytes: defaultBlobQuota,
		},
	})
}

func (h *Handler) handleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB max
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body failed")
		return
	}

	if h.webhookSecret != "" {
		sigHeader := r.Header.Get("Stripe-Signature")
		if err := VerifyWebhookSignature(body, sigHeader, h.webhookSecret, 0); err != nil {
			h.log.Error("webhook signature verification failed", "error", err)
			writeError(w, http.StatusBadRequest, "invalid signature")
			return
		}
	}

	var event StripeEvent
	if err := json.Unmarshal(body, &event); err != nil {
		h.log.Warn("webhook: unparseable event JSON", "error", err)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	subID, status, err := ParseSubscriptionEvent(event)
	if err != nil {
		// Not a subscription event we handle -- acknowledge silently.
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	hh, err := h.store.HouseholdBySubscription(r.Context(), subID)
	if err != nil {
		h.log.Warn("webhook: no household for subscription, ignoring",
			"subscription_id", subID)
		writeJSON(w, http.StatusOK, map[string]string{"status": "ignored"})
		return
	}

	if err := h.store.UpdateSubscription(r.Context(), hh.ID, subID, status); err != nil {
		h.log.Error("webhook: update subscription", "error", err)
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	h.log.Info("subscription updated",
		"household_id", hh.ID,
		"subscription_id", subID,
		"status", status,
	)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("failed to write JSON response", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
