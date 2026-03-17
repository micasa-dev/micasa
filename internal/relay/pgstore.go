// Copyright 2026 Phillip Cloud
// Licensed under the Apache License, Version 2.0

package relay

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cpcloud/micasa/internal/sync"
	"github.com/cpcloud/micasa/internal/uid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// PgStore implements the relay Store interface backed by Postgres via GORM.
type PgStore struct {
	db *gorm.DB
}

// Compile-time interface check.
var _ Store = (*PgStore)(nil)

// GORM models — unexported, internal to PgStore.

type pgHousehold struct {
	ID                   string    `gorm:"primaryKey"`
	SeqCounter           int64     `gorm:"not null;default:0"`
	StripeSubscriptionID string    `gorm:"column:stripe_subscription_id"`
	StripeStatus         string    `gorm:"column:stripe_status"`
	CreatedAt            time.Time `gorm:"not null;autoCreateTime"`
}

func (pgHousehold) TableName() string { return "households" }

type pgDevice struct {
	ID          string     `gorm:"primaryKey"`
	HouseholdID string     `gorm:"not null;index:idx_devices_household"`
	Name        string     `gorm:"not null"`
	PublicKey   []byte     `gorm:"column:public_key"`
	TokenSHA    string     `gorm:"column:token_sha;not null;index:idx_devices_token_sha"`
	LastSeen    *time.Time `gorm:"column:last_seen"`
	CreatedAt   time.Time  `gorm:"not null;autoCreateTime"`
	Revoked     bool       `gorm:"not null;default:false"`
}

func (pgDevice) TableName() string { return "devices" }

type pgOp struct {
	Seq         int64     `gorm:"primaryKey;autoIncrement:false"`
	HouseholdID string    `gorm:"primaryKey"`
	ID          string    `gorm:"column:id;not null;uniqueIndex:idx_ops_dedup,composite:household_id"`
	DeviceID    string    `gorm:"column:device_id;not null"`
	Nonce       []byte    `gorm:"not null;type:bytea"`
	Ciphertext  []byte    `gorm:"not null;type:bytea"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
}

func (pgOp) TableName() string { return "ops" }

type pgInvite struct {
	Code        string    `gorm:"primaryKey"`
	HouseholdID string    `gorm:"not null;index"`
	CreatedBy   string    `gorm:"column:created_by;not null"`
	ExpiresAt   time.Time `gorm:"not null"`
	Consumed    bool      `gorm:"not null;default:false"`
	Attempts    int       `gorm:"not null;default:0"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
}

func (pgInvite) TableName() string { return "invites" }

type pgKeyExchange struct {
	ID                    string    `gorm:"primaryKey"`
	HouseholdID           string    `gorm:"not null;index"`
	InviteCode            string    `gorm:"column:invite_code"`
	JoinerName            string    `gorm:"column:joiner_name"`
	JoinerPublicKey       []byte    `gorm:"column:joiner_public_key"`
	EncryptedHouseholdKey []byte    `gorm:"column:encrypted_household_key"`
	DeviceToken           string    `gorm:"column:device_token"`
	DeviceID              string    `gorm:"column:device_id"`
	CreatedAt             time.Time `gorm:"not null;autoCreateTime"`
	Completed             bool      `gorm:"not null;default:false"`
}

func (pgKeyExchange) TableName() string { return "key_exchanges" }

type pgBlob struct {
	HouseholdID string    `gorm:"primaryKey"`
	Hash        string    `gorm:"primaryKey"`
	Data        []byte    `gorm:"not null;type:bytea"`
	SizeBytes   int64     `gorm:"not null"`
	CreatedAt   time.Time `gorm:"not null;autoCreateTime"`
}

func (pgBlob) TableName() string { return "blobs" }

// OpenPgStore connects to a Postgres database and returns a PgStore.
func OpenPgStore(dsn string) (*PgStore, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	return &PgStore{db: db}, nil
}

// NewPgStore wraps an existing GORM DB as a PgStore (useful for testing).
func NewPgStore(db *gorm.DB) *PgStore {
	return &PgStore{db: db}
}

// pgModels is the canonical list of GORM models managed by PgStore.
// Used by AutoMigrate and tests to avoid maintaining parallel lists.
var pgModels = []any{
	&pgHousehold{},
	&pgDevice{},
	&pgOp{},
	&pgInvite{},
	&pgKeyExchange{},
	&pgBlob{},
}

// AutoMigrate creates/updates the database schema.
func (s *PgStore) AutoMigrate() error {
	return s.db.AutoMigrate(pgModels...)
}

func (s *PgStore) Push(ctx context.Context, ops []sync.Envelope) ([]sync.PushConfirmation, error) {
	confirmed := make([]sync.PushConfirmation, 0, len(ops))

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, op := range ops {
			// Atomic seq increment within the transaction.
			var seq int64
			result := tx.Raw(
				"UPDATE households SET seq_counter = seq_counter + 1 WHERE id = ? RETURNING seq_counter",
				op.HouseholdID,
			).Scan(&seq)
			if result.Error != nil {
				return fmt.Errorf("increment seq for %s: %w", op.HouseholdID, result.Error)
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("household %s not found", op.HouseholdID)
			}

			row := pgOp{
				Seq:         seq,
				HouseholdID: op.HouseholdID,
				ID:          op.ID,
				DeviceID:    op.DeviceID,
				Nonce:       op.Nonce,
				Ciphertext:  op.Ciphertext,
				CreatedAt:   op.CreatedAt,
			}
			if err := tx.Create(&row).Error; err != nil {
				return fmt.Errorf("insert op %s: %w", op.ID, err)
			}

			confirmed = append(confirmed, sync.PushConfirmation{
				ID:  op.ID,
				Seq: seq,
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return confirmed, nil
}

func (s *PgStore) Pull(
	ctx context.Context,
	householdID, excludeDeviceID string,
	afterSeq int64,
	limit int,
) ([]sync.Envelope, bool, error) {
	if limit <= 0 {
		limit = 100
	}

	var rows []pgOp
	q := s.db.WithContext(ctx).
		Where("household_id = ? AND seq > ?", householdID, afterSeq)
	if excludeDeviceID != "" {
		q = q.Where("device_id != ?", excludeDeviceID)
	}
	if err := q.Order("seq ASC").Limit(limit + 1).Find(&rows).Error; err != nil {
		return nil, false, fmt.Errorf("pull ops: %w", err)
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	envs := make([]sync.Envelope, len(rows))
	for i, r := range rows {
		envs[i] = sync.Envelope{
			ID:          r.ID,
			HouseholdID: r.HouseholdID,
			DeviceID:    r.DeviceID,
			Nonce:       r.Nonce,
			Ciphertext:  r.Ciphertext,
			CreatedAt:   r.CreatedAt,
			Seq:         r.Seq,
		}
	}
	return envs, hasMore, nil
}

func (s *PgStore) CreateHousehold(
	ctx context.Context,
	req sync.CreateHouseholdRequest,
) (sync.CreateHouseholdResponse, error) {
	hhID := uid.New()
	devID := uid.New()
	token, tokenHash, err := generateToken()
	if err != nil {
		return sync.CreateHouseholdResponse{}, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		hh := pgHousehold{ID: hhID}
		if err := tx.Create(&hh).Error; err != nil {
			return fmt.Errorf("create household: %w", err)
		}
		dev := pgDevice{
			ID:          devID,
			HouseholdID: hhID,
			Name:        req.DeviceName,
			PublicKey:   req.PublicKey,
			TokenSHA:    tokenHash,
		}
		if err := tx.Create(&dev).Error; err != nil {
			return fmt.Errorf("create device: %w", err)
		}
		return nil
	})
	if err != nil {
		return sync.CreateHouseholdResponse{}, err
	}

	return sync.CreateHouseholdResponse{
		HouseholdID: hhID,
		DeviceID:    devID,
		DeviceToken: token,
	}, nil
}

func (s *PgStore) RegisterDevice(
	ctx context.Context,
	req sync.RegisterDeviceRequest,
) (sync.RegisterDeviceResponse, error) {
	devID := uid.New()
	token, tokenHash, err := generateToken()
	if err != nil {
		return sync.RegisterDeviceResponse{}, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Verify household exists within the transaction.
		var count int64
		if err := tx.Model(&pgHousehold{}).
			Where("id = ?", req.HouseholdID).Count(&count).Error; err != nil {
			return fmt.Errorf("check household: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("household %s not found", req.HouseholdID)
		}

		dev := pgDevice{
			ID:          devID,
			HouseholdID: req.HouseholdID,
			Name:        req.Name,
			PublicKey:   req.PublicKey,
			TokenSHA:    tokenHash,
		}
		if err := tx.Create(&dev).Error; err != nil {
			return fmt.Errorf("create device: %w", err)
		}
		return nil
	})
	if err != nil {
		return sync.RegisterDeviceResponse{}, err
	}

	return sync.RegisterDeviceResponse{
		DeviceID:    devID,
		DeviceToken: token,
	}, nil
}

func (s *PgStore) AuthenticateDevice(ctx context.Context, token string) (sync.Device, error) {
	sha := tokenSHA256(token)

	var dev pgDevice
	result := s.db.WithContext(ctx).Raw(
		"UPDATE devices SET last_seen = now() "+
			"WHERE token_sha = ? AND revoked = false "+
			"RETURNING *",
		sha,
	).Scan(&dev)
	if result.Error != nil {
		return sync.Device{}, fmt.Errorf("authenticate: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return sync.Device{}, fmt.Errorf("invalid token")
	}

	return pgDeviceToSync(dev), nil
}

func (s *PgStore) CreateInvite(
	ctx context.Context,
	householdID, deviceID string,
) (sync.InviteCode, error) {
	code, err := generateInviteCode()
	if err != nil {
		return sync.InviteCode{}, err
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	var result sync.InviteCode

	// Wrap count + create in a transaction with FOR UPDATE to prevent a
	// TOCTOU race where two concurrent requests both see active < max.
	// READ COMMITTED alone is insufficient; the row-level lock serializes
	// concurrent invite creation for the same household.
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Model(&pgInvite{}).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("household_id = ? AND consumed = false AND expires_at > ?",
				householdID, time.Now()).
			Count(&active).Error; err != nil {
			return fmt.Errorf("count invites: %w", err)
		}
		if active >= maxActiveInvites {
			return fmt.Errorf("max active invites reached (%d)", maxActiveInvites)
		}

		inv := pgInvite{
			Code:        code,
			HouseholdID: householdID,
			CreatedBy:   deviceID,
			ExpiresAt:   expiresAt,
		}
		if err := tx.Create(&inv).Error; err != nil {
			return fmt.Errorf("create invite: %w", err)
		}

		result = sync.InviteCode{Code: code, ExpiresAt: expiresAt}
		return nil
	})
	if err != nil {
		return sync.InviteCode{}, err
	}
	return result, nil
}

func (s *PgStore) StartJoin(
	ctx context.Context,
	householdID, code string,
	req sync.JoinRequest,
) (sync.JoinResponse, error) {
	var resp sync.JoinResponse

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var inv pgInvite
		if err := tx.Where("code = ?", code).First(&inv).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("invite code not found")
			}
			return fmt.Errorf("find invite: %w", err)
		}
		if inv.HouseholdID != householdID {
			return fmt.Errorf("invite code not found")
		}
		if inv.Consumed {
			return fmt.Errorf("invite code already consumed")
		}
		if time.Now().After(inv.ExpiresAt) {
			return fmt.Errorf("invite code expired")
		}

		inv.Attempts++
		if inv.Attempts >= maxInviteAttempts {
			inv.Consumed = true
			if err := tx.Save(&inv).Error; err != nil {
				return fmt.Errorf("update invite: %w", err)
			}
			return fmt.Errorf("invite code max attempts exceeded")
		}
		if err := tx.Save(&inv).Error; err != nil {
			return fmt.Errorf("update invite: %w", err)
		}

		// Find inviter's public key.
		var inviterDev pgDevice
		if err := tx.Where("id = ?", inv.CreatedBy).First(&inviterDev).Error; err != nil {
			return fmt.Errorf("inviter device not found")
		}

		exchangeID := newCryptoToken()
		ex := pgKeyExchange{
			ID:              exchangeID,
			HouseholdID:     inv.HouseholdID,
			InviteCode:      code,
			JoinerName:      req.DeviceName,
			JoinerPublicKey: req.PublicKey,
		}
		if err := tx.Create(&ex).Error; err != nil {
			return fmt.Errorf("create key exchange: %w", err)
		}

		resp = sync.JoinResponse{
			ExchangeID:       exchangeID,
			HouseholdID:      inv.HouseholdID,
			InviterPublicKey: inviterDev.PublicKey,
		}
		return nil
	})
	if err != nil {
		return sync.JoinResponse{}, err
	}
	return resp, nil
}

func (s *PgStore) GetPendingExchanges(
	ctx context.Context,
	householdID string,
) ([]sync.PendingKeyExchange, error) {
	var rows []pgKeyExchange
	err := s.db.WithContext(ctx).
		Where("household_id = ? AND completed = false", householdID).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("get pending exchanges: %w", err)
	}

	result := make([]sync.PendingKeyExchange, len(rows))
	for i, r := range rows {
		result[i] = sync.PendingKeyExchange{
			ID:              r.ID,
			JoinerPublicKey: r.JoinerPublicKey,
			JoinerName:      r.JoinerName,
			CreatedAt:       r.CreatedAt,
		}
	}
	return result, nil
}

func (s *PgStore) CompleteKeyExchange(
	ctx context.Context,
	householdID, exchangeID string,
	encryptedKey []byte,
) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ex pgKeyExchange
		if err := tx.Where("id = ?", exchangeID).First(&ex).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("key exchange %s not found", exchangeID)
			}
			return fmt.Errorf("find exchange: %w", err)
		}
		if ex.HouseholdID != householdID {
			return fmt.Errorf("key exchange does not belong to this household")
		}
		if ex.Completed {
			return fmt.Errorf("key exchange already completed")
		}

		// Register the joiner as a device.
		devID := uid.New()
		token, tokenHash, err := generateToken()
		if err != nil {
			return err
		}

		dev := pgDevice{
			ID:          devID,
			HouseholdID: householdID,
			Name:        ex.JoinerName,
			PublicKey:   ex.JoinerPublicKey,
			TokenSHA:    tokenHash,
		}
		if err := tx.Create(&dev).Error; err != nil {
			return fmt.Errorf("create joiner device: %w", err)
		}

		ex.EncryptedHouseholdKey = encryptedKey
		ex.DeviceID = devID
		ex.DeviceToken = token
		ex.Completed = true
		if err := tx.Save(&ex).Error; err != nil {
			return fmt.Errorf("complete exchange: %w", err)
		}

		// Consume the invite code.
		if err := tx.Model(&pgInvite{}).
			Where("code = ?", ex.InviteCode).
			Update("consumed", true).Error; err != nil {
			return fmt.Errorf("consume invite: %w", err)
		}

		return nil
	})
}

func (s *PgStore) GetKeyExchangeResult(
	ctx context.Context,
	exchangeID string,
) (sync.KeyExchangeResult, error) {
	var result sync.KeyExchangeResult
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ex pgKeyExchange
		if err := tx.Clauses(clause.Locking{Strength: clause.LockingStrengthUpdate}).
			Where("id = ?", exchangeID).First(&ex).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("key exchange %s not found", exchangeID)
			}
			return fmt.Errorf("get exchange: %w", err)
		}

		if !ex.Completed {
			result = sync.KeyExchangeResult{Ready: false}
			return nil
		}

		result = sync.KeyExchangeResult{
			Ready:                 true,
			EncryptedHouseholdKey: ex.EncryptedHouseholdKey,
			DeviceID:              ex.DeviceID,
			DeviceToken:           ex.DeviceToken,
		}

		// Single-use: clear credentials atomically within this transaction.
		if err := tx.Model(&pgKeyExchange{}).
			Where("id = ?", exchangeID).
			Updates(map[string]any{
				"encrypted_household_key": nil,
				"device_token":            "",
			}).Error; err != nil {
			return fmt.Errorf("clear exchange credentials: %w", err)
		}

		return nil
	})
	return result, err
}

func (s *PgStore) ListDevices(
	ctx context.Context,
	householdID string,
) ([]sync.Device, error) {
	var rows []pgDevice
	err := s.db.WithContext(ctx).
		Where("household_id = ? AND revoked = false", householdID).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list devices: %w", err)
	}

	result := make([]sync.Device, len(rows))
	for i, r := range rows {
		result[i] = pgDeviceToSync(r)
	}
	return result, nil
}

func (s *PgStore) RevokeDevice(ctx context.Context, householdID, deviceID string) error {
	var dev pgDevice
	err := s.db.WithContext(ctx).Where("id = ?", deviceID).First(&dev).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("device %s not found", deviceID)
		}
		return fmt.Errorf("find device: %w", err)
	}
	if dev.HouseholdID != householdID {
		return fmt.Errorf("device does not belong to this household")
	}

	return s.db.WithContext(ctx).Model(&pgDevice{}).
		Where("id = ?", deviceID).
		Update("revoked", true).Error
}

func (s *PgStore) GetHousehold(
	ctx context.Context,
	householdID string,
) (sync.Household, error) {
	var hh pgHousehold
	err := s.db.WithContext(ctx).Where("id = ?", householdID).First(&hh).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sync.Household{}, fmt.Errorf("household %s not found", householdID)
		}
		return sync.Household{}, fmt.Errorf("get household: %w", err)
	}
	return pgHouseholdToSync(hh), nil
}

func (s *PgStore) UpdateSubscription(
	ctx context.Context,
	householdID, subscriptionID, status string,
) error {
	result := s.db.WithContext(ctx).Model(&pgHousehold{}).
		Where("id = ?", householdID).
		Updates(map[string]any{
			"stripe_subscription_id": subscriptionID,
			"stripe_status":          status,
		})
	if result.Error != nil {
		return fmt.Errorf("update subscription: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("household %s not found", householdID)
	}
	return nil
}

func (s *PgStore) HouseholdBySubscription(
	ctx context.Context,
	subscriptionID string,
) (sync.Household, error) {
	var hh pgHousehold
	err := s.db.WithContext(ctx).
		Where("stripe_subscription_id = ?", subscriptionID).
		First(&hh).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return sync.Household{}, fmt.Errorf("no household with subscription %s", subscriptionID)
		}
		return sync.Household{}, fmt.Errorf("find household by subscription: %w", err)
	}
	return pgHouseholdToSync(hh), nil
}

func (s *PgStore) OpsCount(ctx context.Context, householdID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&pgOp{}).
		Where("household_id = ?", householdID).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("count ops: %w", err)
	}
	return count, nil
}

func (s *PgStore) PutBlob(ctx context.Context, householdID, hash string, data []byte) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if blob already exists.
		var count int64
		if err := tx.Model(&pgBlob{}).
			Where("household_id = ? AND hash = ?", householdID, hash).
			Count(&count).Error; err != nil {
			return fmt.Errorf("check blob: %w", err)
		}
		if count > 0 {
			return errBlobExists
		}

		// Check quota. Written as used > quota-len to avoid overflow on
		// the left side (used+len could wrap). Safe with signed int64:
		// if len(data) > quota the RHS goes negative and the check holds.
		var used int64
		if err := tx.Model(&pgBlob{}).
			Where("household_id = ?", householdID).
			Select("COALESCE(SUM(size_bytes), 0)").
			Scan(&used).Error; err != nil {
			return fmt.Errorf("check quota: %w", err)
		}
		if used > defaultBlobQuota-int64(len(data)) {
			return errQuotaExceeded
		}

		blob := pgBlob{
			HouseholdID: householdID,
			Hash:        hash,
			Data:        data,
			SizeBytes:   int64(len(data)),
		}
		if err := tx.Create(&blob).Error; err != nil {
			return fmt.Errorf("store blob: %w", err)
		}
		return nil
	})
}

func (s *PgStore) GetBlob(ctx context.Context, householdID, hash string) ([]byte, error) {
	var blob pgBlob
	err := s.db.WithContext(ctx).
		Where("household_id = ? AND hash = ?", householdID, hash).
		First(&blob).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errBlobNotFound
		}
		return nil, fmt.Errorf("get blob: %w", err)
	}
	return blob.Data, nil
}

func (s *PgStore) HasBlob(ctx context.Context, householdID, hash string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&pgBlob{}).
		Where("household_id = ? AND hash = ?", householdID, hash).
		Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("check blob: %w", err)
	}
	return count > 0, nil
}

func (s *PgStore) BlobUsage(ctx context.Context, householdID string) (int64, error) {
	var used int64
	err := s.db.WithContext(ctx).Model(&pgBlob{}).
		Where("household_id = ?", householdID).
		Select("COALESCE(SUM(size_bytes), 0)").
		Scan(&used).Error
	if err != nil {
		return 0, fmt.Errorf("blob usage: %w", err)
	}
	return used, nil
}

func (s *PgStore) Close() error {
	db, err := s.db.DB()
	if err != nil {
		return err
	}
	return db.Close()
}

// pgDeviceToSync converts a pgDevice to a sync.Device.
func pgDeviceToSync(d pgDevice) sync.Device {
	return sync.Device{
		ID:          d.ID,
		HouseholdID: d.HouseholdID,
		Name:        d.Name,
		PublicKey:   d.PublicKey,
		CreatedAt:   d.CreatedAt,
	}
}

// pgHouseholdToSync converts a pgHousehold to a sync.Household.
func pgHouseholdToSync(h pgHousehold) sync.Household {
	return sync.Household{
		ID:                   h.ID,
		CreatedAt:            h.CreatedAt,
		StripeSubscriptionID: h.StripeSubscriptionID,
		StripeStatus:         h.StripeStatus,
	}
}
