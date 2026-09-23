package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

const pairingBootstrapProofBytes = 32

func newPairingBootstrapProof() (string, []byte, error) {
	random := make([]byte, pairingBootstrapProofBytes)
	if _, err := rand.Read(random); err != nil {
		return "", []byte{}, fmt.Errorf("generating pairing bootstrap proof: %w", err)
	}

	proof := base64.RawURLEncoding.EncodeToString(random)
	return proof, hashPairingBootstrapProof(proof), nil
}

func hashPairingBootstrapProof(proof string) []byte {
	sum := sha256.Sum256([]byte(proof))
	hash := make([]byte, len(sum))
	copy(hash, sum[:])
	return hash
}

// PairingSessionByBootstrapProof resolves an unexpired session for its claiming device.
func (s *Store) PairingSessionByBootstrapProof(
	ctx context.Context,
	sessionID string,
	proof string,
) (PairingSession, error) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(proof) == "" {
		return PairingSession{}, ErrNotFound
	}

	var customerID CustomerID
	storedHash := []byte{}
	err := s.db.QueryRowContext(
		ctx,
		"SELECT customer_id, claim_proof_hash FROM pairing_sessions WHERE id = ?",
		sessionID,
	).Scan(&customerID, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return PairingSession{}, ErrNotFound
	}
	if err != nil {
		return PairingSession{}, fmt.Errorf("reading pairing bootstrap proof: %w", err)
	}

	candidateHash := hashPairingBootstrapProof(proof)
	if subtle.ConstantTimeCompare(storedHash, candidateHash) != 1 {
		return PairingSession{}, ErrNotFound
	}
	return s.PairingSession(ctx, customerID, sessionID)
}
