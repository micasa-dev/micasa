<!-- Copyright 2026 Phillip Cloud -->
<!-- Licensed under the Apache License, Version 2.0 -->

# Phase 7: Payment & Launch

## Goal

Gate sync behind an active Stripe subscription. Process Stripe webhooks
to track subscription status. Return 402 when household subscription is
inactive.

## Implementation Plan

### Step 1: Subscription fields on Household

Add `StripeSubscriptionID` and `StripeStatus` to the Household type.
Valid statuses: `active`, `past_due`, `canceled`, empty (no subscription).

### Step 2: Store interface + MemStore

- `UpdateSubscription(ctx, householdID, subscriptionID, status)` -- set Stripe fields
- `GetHousehold(ctx, householdID)` -- retrieve household with status
- MemStore defaults new households to `StripeStatus = "active"` for dev/test

### Step 3: Subscription gating middleware

On push/pull, check household's `StripeStatus`. If not `"active"`,
return 402 with message "subscription inactive". Skip check when status
is empty (free/dev mode).

### Step 4: Stripe webhook handler

`POST /webhooks/stripe` -- receives Stripe subscription events:
- Verifies webhook signature (HMAC-SHA256, no stripe-go dependency)
- Parses event type and subscription data from JSON
- Updates household subscription status via Store

Supported events:
- `customer.subscription.created` -- set active
- `customer.subscription.updated` -- update status
- `customer.subscription.deleted` -- set canceled

### Step 5: Status endpoint

`GET /status` (auth required) -- returns household sync status, device
count, and subscription status.

### Step 6: Tests

- Push/pull return 402 when subscription canceled
- Push/pull succeed when subscription active
- Webhook updates subscription status
- Webhook rejects invalid signatures
- Status endpoint returns correct info

## File Changes

| File | Change |
|------|--------|
| `internal/sync/types.go` | Extended: subscription fields on Household |
| `internal/relay/store.go` | Extended: subscription + household methods |
| `internal/relay/memstore.go` | Extended: subscription implementation |
| `internal/relay/stripe.go` | New: webhook signature verification |
| `internal/relay/handler.go` | Extended: gating, webhook, status routes |
| `internal/relay/handler_test.go` | Extended: payment tests |
| `internal/relay/stripe_test.go` | New: signature verification tests |
