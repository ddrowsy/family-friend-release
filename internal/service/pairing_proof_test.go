package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestDevicePairingClaimReturnsBootstrapProof(t *testing.T) {
	ctx := context.Background()
	store := newPairingStore(t, ctx)
	now := time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
	store.now = func() time.Time { return now }

	session, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"123456",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession: %v", err)
	}

	response := serveHTTP(
		t,
		NewHTTPHandler(store),
		testHTTPRequest{
			method: http.MethodPost,
			path:   devicePairingClaimRoute,
			body: DevicePairingClaimRequest{
				Code:       "123456",
				DeviceID:   "device-1",
				DeviceName: "Harry Laptop",
				Platform:   "linux",
			},
		},
	)
	assertStatus(t, response, http.StatusOK)

	var result DevicePairingClaimResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode claim response: %v", err)
	}
	decodedProof, err := base64.RawURLEncoding.DecodeString(result.ClaimProof)
	if err != nil {
		t.Fatalf("decode claim proof: %v", err)
	}
	if len(decodedProof) != pairingBootstrapProofBytes {
		t.Fatalf("claim proof bytes = %d, want %d", len(decodedProof), pairingBootstrapProofBytes)
	}

	storedHash := []byte{}
	if err := store.db.QueryRowContext(
		ctx,
		"SELECT claim_proof_hash FROM pairing_sessions WHERE id = ?",
		session.ID,
	).Scan(&storedHash); err != nil {
		t.Fatalf("read persisted claim proof hash: %v", err)
	}
	if bytes.Equal(storedHash, []byte(result.ClaimProof)) {
		t.Fatal("plaintext claim proof was persisted")
	}
	if !bytes.Equal(storedHash, hashPairingBootstrapProof(result.ClaimProof)) {
		t.Fatal("persisted claim proof hash does not match response proof")
	}

	resolved, err := store.PairingSessionByBootstrapProof(
		ctx,
		session.ID,
		result.ClaimProof,
	)
	if err != nil {
		t.Fatalf("PairingSessionByBootstrapProof: %v", err)
	}
	if resolved.ID != session.ID || resolved.DeviceID != "device-1" {
		t.Fatalf("resolved session = %#v", resolved)
	}
	if _, err := store.PairingSessionByBootstrapProof(
		ctx,
		session.ID,
		"wrong-proof",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong proof error = %v, want ErrNotFound", err)
	}

	now = now.Add(10*time.Minute + time.Second)
	if _, err := store.PairingSessionByBootstrapProof(
		ctx,
		session.ID,
		result.ClaimProof,
	); !errors.Is(err, ErrPairingSessionExpired) {
		t.Fatalf("expired proof error = %v, want ErrPairingSessionExpired", err)
	}
}

func TestPairingBootstrapProofDoesNotCrossSessions(t *testing.T) {
	ctx := context.Background()
	store := newPairingStoreWithCustomers(
		t,
		ctx,
		"customer-1",
		"customer-2",
	)
	now := time.Date(
		2026,
		time.September,
		22,
		14,
		0,
		0,
		0,
		time.UTC,
	)
	store.now = func() time.Time { return now }

	first, err := store.CreatePairingSession(
		ctx,
		"customer-1",
		"111111",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession first: %v", err)
	}
	if _, err := store.ClaimPairingSession(
		ctx,
		"111111",
		PairingDeviceClaim{DeviceID: "device-1"},
		hashPairingBootstrapProof("proof-one"),
	); err != nil {
		t.Fatalf("ClaimPairingSession first: %v", err)
	}

	second, err := store.CreatePairingSession(
		ctx,
		"customer-2",
		"222222",
		now.Add(35*time.Second),
	)
	if err != nil {
		t.Fatalf("CreatePairingSession second: %v", err)
	}
	if _, err := store.ClaimPairingSession(
		ctx,
		"222222",
		PairingDeviceClaim{DeviceID: "device-2"},
		hashPairingBootstrapProof("proof-two"),
	); err != nil {
		t.Fatalf("ClaimPairingSession second: %v", err)
	}

	if _, err := store.PairingSessionByBootstrapProof(
		ctx,
		second.ID,
		"proof-one",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-session proof error = %v, want ErrNotFound", err)
	}
	if _, err := store.PairingSessionByBootstrapProof(
		ctx,
		first.ID,
		"proof-two",
	); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reverse cross-session proof error = %v, want ErrNotFound", err)
	}
}
