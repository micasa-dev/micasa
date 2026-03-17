// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeSignatureHeader(payload []byte, secret string, ts time.Time) string {
	signed := fmt.Sprintf("%d.%s", ts.Unix(), string(payload))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%d,v1=%s", ts.Unix(), sig)
}

func TestVerifyWebhookSignatureValid(t *testing.T) {
	t.Parallel()
	secret := "whsec_test_secret"
	payload := []byte(`{"type":"customer.subscription.created"}`)
	header := makeSignatureHeader(payload, secret, time.Now())

	err := VerifyWebhookSignature(payload, header, secret, 5*time.Minute)
	assert.NoError(t, err)
}

func TestVerifyWebhookSignatureWrongSecret(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"type":"test"}`)
	header := makeSignatureHeader(payload, "correct-secret", time.Now())

	err := VerifyWebhookSignature(payload, header, "wrong-secret", 5*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no matching signature")
}

func TestVerifyWebhookSignatureExpired(t *testing.T) {
	t.Parallel()
	secret := "whsec_test"
	payload := []byte(`{"type":"test"}`)
	header := makeSignatureHeader(payload, secret, time.Now().Add(-10*time.Minute))

	err := VerifyWebhookSignature(payload, header, secret, 5*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too old")
}

func TestVerifyWebhookSignatureInvalidHeader(t *testing.T) {
	t.Parallel()
	err := VerifyWebhookSignature([]byte("body"), "garbage", "secret", 5*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid signature header")
}

func TestVerifyWebhookSignatureTamperedPayload(t *testing.T) {
	t.Parallel()
	secret := "whsec_test"
	original := []byte(`{"type":"test"}`)
	header := makeSignatureHeader(original, secret, time.Now())

	tampered := []byte(`{"type":"tampered"}`)
	err := VerifyWebhookSignature(tampered, header, secret, 5*time.Minute)
	assert.Error(t, err)
}

func TestParseSubscriptionEvent(t *testing.T) {
	t.Parallel()
	data, _ := json.Marshal(StripeSubscriptionData{
		Object: struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		}{
			ID:     "sub_12345",
			Status: "active",
		},
	})
	event := StripeEvent{
		ID:   "evt_1",
		Type: "customer.subscription.created",
		Data: data,
	}

	subID, status, err := ParseSubscriptionEvent(event)
	require.NoError(t, err)
	assert.Equal(t, "sub_12345", subID)
	assert.Equal(t, "active", status)
}

func TestParseSubscriptionEventUnsupportedType(t *testing.T) {
	t.Parallel()
	event := StripeEvent{Type: "charge.succeeded", Data: json.RawMessage(`{}`)}
	_, _, err := ParseSubscriptionEvent(event)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported")
}

func TestVerifyWebhookSignatureFutureTimestamp(t *testing.T) {
	t.Parallel()
	secret := "whsec_test"
	payload := []byte(`{"type":"test"}`)
	header := makeSignatureHeader(payload, secret, time.Now().Add(10*time.Minute))

	err := VerifyWebhookSignature(payload, header, secret, 5*time.Minute)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "future")
}
