// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync_test

import (
	"context"
	"testing"

	"github.com/micasa-dev/micasa/internal/crypto"
	"github.com/micasa-dev/micasa/internal/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCreateHousehold(t *testing.T) {
	t.Parallel()
	srv, _, _ := setupTestRelay(t)

	client := sync.NewManagementClient(srv.URL, "")
	kp, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	resp, err := client.CreateHousehold(sync.CreateHouseholdRequest{
		DeviceName: "my-laptop",
		PublicKey:  kp.PublicKey[:],
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.HouseholdID)
	assert.NotEmpty(t, resp.DeviceID)
	assert.NotEmpty(t, resp.DeviceToken)
}

func TestClientStatus(t *testing.T) {
	t.Parallel()
	srv, _, token := setupTestRelay(t)

	client := sync.NewManagementClient(srv.URL, token)
	status, err := client.Status()
	require.NoError(t, err)
	assert.NotEmpty(t, status.HouseholdID)
	assert.NotEmpty(t, status.Devices)
}

func TestClientInviteAndJoin(t *testing.T) {
	t.Parallel()
	srv, _, tokenA := setupTestRelay(t)

	// Get household ID from status.
	clientA := sync.NewManagementClient(srv.URL, tokenA)
	status, err := clientA.Status()
	require.NoError(t, err)

	// Create invite.
	invite, err := clientA.Invite(status.HouseholdID)
	require.NoError(t, err)
	assert.NotEmpty(t, invite.Code)
	assert.False(t, invite.ExpiresAt.IsZero())

	// Joiner creates a keypair and joins.
	kpB, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	joinerClient := sync.NewManagementClient(srv.URL, "")
	joinResp, err := joinerClient.Join(status.HouseholdID, sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "joiner-device",
		PublicKey:  kpB.PublicKey[:],
	})
	require.NoError(t, err)
	assert.NotEmpty(t, joinResp.ExchangeID)
	assert.NotEmpty(t, joinResp.InviterPublicKey)
}

func TestClientListDevices(t *testing.T) {
	t.Parallel()
	srv, _, token := setupTestRelay(t)

	client := sync.NewManagementClient(srv.URL, token)
	status, err := client.Status()
	require.NoError(t, err)

	devices, err := client.ListDevices(status.HouseholdID)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
	assert.Equal(t, "test-device", devices[0].Name)
}

func TestClientGetPendingExchanges(t *testing.T) {
	t.Parallel()
	srv, _, token := setupTestRelay(t)

	client := sync.NewManagementClient(srv.URL, token)
	status, err := client.Status()
	require.NoError(t, err)

	exchanges, err := client.GetPendingExchanges(status.HouseholdID)
	require.NoError(t, err)
	assert.Empty(t, exchanges)
}

func TestClientCompleteKeyExchangeAndGetResult(t *testing.T) {
	t.Parallel()
	srv, _, tokenA := setupTestRelay(t)

	clientA := sync.NewManagementClient(srv.URL, tokenA)
	status, err := clientA.Status()
	require.NoError(t, err)

	// Create invite.
	invite, err := clientA.Invite(status.HouseholdID)
	require.NoError(t, err)

	// Joiner joins.
	kpB, err := crypto.GenerateDeviceKeyPair()
	require.NoError(t, err)

	joinerClient := sync.NewManagementClient(srv.URL, "")
	joinResp, err := joinerClient.Join(status.HouseholdID, sync.JoinRequest{
		InviteCode: invite.Code,
		DeviceName: "joiner",
		PublicKey:  kpB.PublicKey[:],
	})
	require.NoError(t, err)

	// Inviter sees pending exchange.
	exchanges, err := clientA.GetPendingExchanges(status.HouseholdID)
	require.NoError(t, err)
	require.Len(t, exchanges, 1)
	assert.Equal(t, joinResp.ExchangeID, exchanges[0].ID)

	// Inviter completes key exchange.
	fakeEncryptedKey := []byte("encrypted-household-key-here!!!")
	err = clientA.CompleteKeyExchange(joinResp.ExchangeID, fakeEncryptedKey)
	require.NoError(t, err)

	// Joiner polls for result.
	result, err := joinerClient.GetKeyExchangeResult(joinResp.ExchangeID)
	require.NoError(t, err)
	assert.True(t, result.Ready)
	assert.NotEmpty(t, result.DeviceID)
	assert.NotEmpty(t, result.DeviceToken)
	assert.Equal(t, fakeEncryptedKey, result.EncryptedHouseholdKey)
}

func TestClientRevokeDevice(t *testing.T) {
	t.Parallel()
	srv, store, tokenA := setupTestRelay(t)

	clientA := sync.NewManagementClient(srv.URL, tokenA)
	status, err := clientA.Status()
	require.NoError(t, err)

	// Register a second device to revoke.
	regResp, err := store.RegisterDevice(context.Background(), sync.RegisterDeviceRequest{
		HouseholdID: status.HouseholdID,
		Name:        "second-device",
	})
	require.NoError(t, err)

	// Revoke it.
	err = clientA.RevokeDevice(status.HouseholdID, regResp.DeviceID)
	require.NoError(t, err)

	// Should only have 1 device now.
	devices, err := clientA.ListDevices(status.HouseholdID)
	require.NoError(t, err)
	assert.Len(t, devices, 1)
}

func TestHouseholdURLHandlesTrailingSlash(t *testing.T) {
	t.Parallel()
	srv, _, tokenA := setupTestRelay(t)

	// Use trailing slash in base URL.
	clientA := sync.NewManagementClient(srv.URL+"/", tokenA)
	status, err := clientA.Status()
	require.NoError(t, err)
	assert.NotEmpty(t, status.HouseholdID)

	// Invite should also work with trailing slash.
	invite, err := clientA.Invite(status.HouseholdID)
	require.NoError(t, err)
	assert.NotEmpty(t, invite.Code)

	// ListDevices too.
	devices, err := clientA.ListDevices(status.HouseholdID)
	require.NoError(t, err)
	assert.NotEmpty(t, devices)
}

func TestClientStatusUnauthorized(t *testing.T) {
	t.Parallel()
	srv, _, _ := setupTestRelay(t)

	client := sync.NewManagementClient(srv.URL, "bad-token")
	_, err := client.Status()
	assert.Error(t, err)
}
