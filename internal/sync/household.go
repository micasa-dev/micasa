// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package sync

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CreateHousehold registers a new household and the founding device.
// No auth token is required.
func (c *Client) CreateHousehold(req CreateHouseholdRequest) (*CreateHouseholdResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal create household request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", c.baseURL+"/households", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("create household request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create household failed (status %d): %s", resp.StatusCode, respBody)
	}

	var result CreateHouseholdResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode create household response: %w", err)
	}
	return &result, nil
}

// Invite creates a one-time invite code for the household.
func (c *Client) Invite(householdID string) (*InviteCode, error) {
	httpReq, err := http.NewRequest("POST", c.baseURL+"/households/"+householdID+"/invite", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("invite request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("invite failed (status %d): %s", resp.StatusCode, respBody)
	}

	var result InviteCode
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode invite response: %w", err)
	}
	return &result, nil
}

// Join initiates a join request with an invite code. No auth required.
func (c *Client) Join(householdID string, req JoinRequest) (*JoinResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal join request: %w", err)
	}

	httpReq, err := http.NewRequest(
		"POST",
		c.baseURL+"/households/"+householdID+"/join",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("join request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("join failed (status %d): %s", resp.StatusCode, respBody)
	}

	var result JoinResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode join response: %w", err)
	}
	return &result, nil
}

// Status returns the sync status for the authenticated device's household.
func (c *Client) Status() (*StatusResponse, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("status request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status failed (status %d): %s", resp.StatusCode, respBody)
	}

	var result StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode status response: %w", err)
	}
	return &result, nil
}

// ListDevices returns all devices in the household.
func (c *Client) ListDevices(householdID string) ([]Device, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/households/"+householdID+"/devices", nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("list devices request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list devices failed (status %d): %s", resp.StatusCode, respBody)
	}

	var result struct {
		Devices []Device `json:"devices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode list devices response: %w", err)
	}
	return result.Devices, nil
}

// RevokeDevice removes a device from the household.
func (c *Client) RevokeDevice(householdID, deviceID string) error {
	httpReq, err := http.NewRequest(
		"DELETE",
		c.baseURL+"/households/"+householdID+"/devices/"+deviceID,
		nil,
	)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("revoke device request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("revoke device failed (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// GetPendingExchanges returns incomplete key exchanges for the household.
func (c *Client) GetPendingExchanges(householdID string) ([]PendingKeyExchange, error) {
	httpReq, err := http.NewRequest(
		"GET",
		c.baseURL+"/households/"+householdID+"/pending-exchanges",
		nil,
	)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("pending exchanges request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"pending exchanges failed (status %d): %s",
			resp.StatusCode,
			respBody,
		)
	}

	var result struct {
		Exchanges []PendingKeyExchange `json:"exchanges"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode pending exchanges response: %w", err)
	}
	return result.Exchanges, nil
}

// CompleteKeyExchange sends the encrypted household key for a pending exchange.
func (c *Client) CompleteKeyExchange(exchangeID string, encryptedKey []byte) error {
	body, err := json.Marshal(CompleteKeyExchangeRequest{
		EncryptedHouseholdKey: encryptedKey,
	})
	if err != nil {
		return fmt.Errorf("marshal complete key exchange request: %w", err)
	}

	httpReq, err := http.NewRequest(
		"POST",
		c.baseURL+"/key-exchange/"+exchangeID+"/complete",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("complete key exchange request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("complete key exchange failed (status %d): %s", resp.StatusCode, respBody)
	}
	return nil
}

// GetKeyExchangeResult polls the key exchange status for a joiner.
// This endpoint is intentionally unauthenticated: the joiner does not
// yet have a device token. The exchange ID (a high-entropy ULID) serves
// as a bearer credential -- it is only known to the inviter and joiner.
func (c *Client) GetKeyExchangeResult(exchangeID string) (*KeyExchangeResult, error) {
	httpReq, err := http.NewRequest("GET", c.baseURL+"/key-exchange/"+exchangeID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("key exchange result request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"key exchange result failed (status %d): %s",
			resp.StatusCode,
			respBody,
		)
	}

	var result KeyExchangeResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode key exchange result: %w", err)
	}
	return &result, nil
}
