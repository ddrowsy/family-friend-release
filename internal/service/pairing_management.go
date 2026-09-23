package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"
)

const (
	pairingWaitingLifetime        = 35 * time.Second
	pairingRotationThreshold      = 5 * time.Second
	pairingCodeGenerationAttempts = 10
	pairingCodeUpperBound         = 1_000_000
)

var errPairingCodeUnavailable = errors.New("pairing code unavailable")

type managementPairingService struct {
	store        *Store
	generateCode func() (string, error)
}

func newManagementPairingService(store *Store) *managementPairingService {
	return &managementPairingService{
		store:        store,
		generateCode: generatePairingCode,
	}
}

func (s *managementPairingService) create(
	ctx context.Context,
	customerID CustomerID,
) (PairingSession, error) {
	code, err := s.nextAvailableCode(ctx)
	if err != nil {
		return PairingSession{}, err
	}

	now := s.store.now().UTC()
	return s.store.CreatePairingSession(
		ctx,
		customerID,
		code,
		now.Add(pairingWaitingLifetime),
	)
}

func (s *managementPairingService) heartbeat(
	ctx context.Context,
	customerID CustomerID,
	sessionID string,
) (PairingSession, error) {
	session, err := s.store.PairingSession(ctx, customerID, sessionID)
	if err != nil {
		return PairingSession{}, err
	}

	switch session.State {
	case PairingStatePendingConfirmation:
		return session, nil
	case PairingStateWaiting:
	default:
		return PairingSession{}, ErrPairingSessionState
	}

	now := s.store.now().UTC()
	if session.ExpiresAt.Sub(now) > pairingRotationThreshold {
		return session, nil
	}

	code, err := s.nextAvailableCode(ctx)
	if err != nil {
		return PairingSession{}, err
	}
	return s.store.RotatePairingSessionCode(
		ctx,
		customerID,
		sessionID,
		PairingCodeRotation{
			Code:      code,
			ExpiresAt: now.Add(pairingWaitingLifetime),
		},
	)
}

func (s *managementPairingService) nextAvailableCode(
	ctx context.Context,
) (string, error) {
	for range pairingCodeGenerationAttempts {
		code, err := s.generateCode()
		if err != nil {
			return "", fmt.Errorf("generating pairing code: %w", err)
		}

		_, err = s.store.PairingSessionByCode(ctx, code)
		switch {
		case errors.Is(err, ErrNotFound):
			return code, nil
		case err == nil:
			continue
		case errors.Is(err, ErrPairingSessionExpired):
			continue
		case errors.Is(err, ErrPairingSessionState):
			continue
		default:
			return "", fmt.Errorf("checking pairing code availability: %w", err)
		}
	}
	return "", errPairingCodeUnavailable
}

func generatePairingCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(pairingCodeUpperBound))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}
