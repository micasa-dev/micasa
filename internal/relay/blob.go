// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"errors"
	"regexp"
)

// Blob error sentinels.
var (
	errBlobExists    = errors.New("blob already exists")
	errBlobNotFound  = errors.New("blob not found")
	errQuotaExceeded = errors.New("blob storage quota exceeded")
)

// DefaultBlobQuota is the included blob storage per household (1 GB).
// Used as the cloud-mode default when no WithBlobQuota option is set.
const DefaultBlobQuota int64 = 1 << 30

// maxBlobSize is the maximum size of a single blob upload (50 MB).
const maxBlobSize int64 = 50 << 20

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// validSHA256Hash returns true if s is a lowercase hex-encoded SHA-256 hash.
func validSHA256Hash(s string) bool {
	return sha256Re.MatchString(s)
}
