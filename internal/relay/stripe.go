// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// VerifyWebhookSignature checks a Stripe webhook signature header against
// the raw request body and signing secret. Stripe's scheme:
// header = "t=<unix_timestamp>,v1=<hmac_sha256(timestamp.payload, secret)>"
//
// Rejects signatures older than tolerance (default 5 minutes).
func VerifyWebhookSignature(
	payload []byte,
	sigHeader, secret string,
	tolerance time.Duration,
) error {
	if tolerance == 0 {
		tolerance = 5 * time.Minute
	}

	parts := parseSignatureHeader(sigHeader)
	if parts.timestamp == "" || len(parts.signatures) == 0 {
		return fmt.Errorf("invalid signature header format")
	}

	ts, err := strconv.ParseInt(parts.timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid timestamp: %w", err)
	}

	sigTime := time.Unix(ts, 0)
	if time.Since(sigTime) > tolerance {
		return fmt.Errorf("signature timestamp too old")
	}
	if time.Until(sigTime) > tolerance {
		return fmt.Errorf("signature timestamp too far in the future")
	}

	// Compute expected signature: HMAC-SHA256(timestamp + "." + payload, secret)
	signed := parts.timestamp + "." + string(payload)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	expected := hex.EncodeToString(mac.Sum(nil))

	for _, sig := range parts.signatures {
		if hmac.Equal([]byte(sig), []byte(expected)) {
			return nil
		}
	}
	return fmt.Errorf("no matching signature found")
}

type signatureParts struct {
	timestamp  string
	signatures []string
}

func parseSignatureHeader(header string) signatureParts {
	var parts signatureParts
	for item := range strings.SplitSeq(header, ",") {
		kv := strings.SplitN(item, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			parts.timestamp = kv[1]
		case "v1":
			parts.signatures = append(parts.signatures, kv[1])
		}
	}
	return parts
}

// StripeEvent represents a parsed Stripe webhook event.
type StripeEvent struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// StripeSubscriptionData holds the subscription fields we care about.
type StripeSubscriptionData struct {
	Object struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	} `json:"object"`
}

// ParseSubscriptionEvent extracts subscription ID and status from a
// Stripe webhook event. Returns an error if the event type is not a
// subscription event or the data can't be parsed.
func ParseSubscriptionEvent(event StripeEvent) (subscriptionID, status string, err error) {
	switch event.Type {
	case "customer.subscription.created",
		"customer.subscription.updated",
		"customer.subscription.deleted":
	default:
		return "", "", fmt.Errorf("unsupported event type: %s", event.Type)
	}

	var data StripeSubscriptionData
	if err := json.Unmarshal(event.Data, &data); err != nil {
		return "", "", fmt.Errorf("parse subscription data: %w", err)
	}
	return data.Object.ID, data.Object.Status, nil
}
