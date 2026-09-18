package streamtoken

import (
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	s, err := NewSigner("a-very-long-secret-that-is-cryptographically-random")
	if err != nil {
		t.Fatalf("NewSigner: %v", err)
	}

	tok, err := s.Sign("device-123", "user-456", time.Minute)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	payload, err := s.Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if payload.DeviceID != "device-123" {
		t.Errorf("DeviceID = %q, want device-123", payload.DeviceID)
	}
	if payload.UserID != "user-456" {
		t.Errorf("UserID = %q, want user-456", payload.UserID)
	}
}

func TestVerifyTampered(t *testing.T) {
	s, _ := NewSigner("some-other-but-still-long-secret")
	tok, _ := s.Sign("d", "u", time.Minute)
	if _, err := s.Verify(tok + "x"); err != ErrSignature {
		t.Errorf("Expected ErrSignature on tampered token, got %v", err)
	}
}

func TestVerifyExpired(t *testing.T) {
	s, _ := NewSigner("some-other-but-still-long-secret")
	tok, _ := s.Sign("d", "u", -time.Minute) // already expired
	if _, err := s.Verify(tok); err != ErrExpired {
		t.Errorf("Expected ErrExpired, got %v", err)
	}
}

func TestVerifyMalformed(t *testing.T) {
	s, _ := NewSigner("some-other-but-still-long-secret")
	if _, err := s.Verify("no-signature"); err != ErrMalformed {
		t.Errorf("Expected ErrMalformed, got %v", err)
	}
}

func TestVerifyWrongKey(t *testing.T) {
	issuer, _ := NewSigner("issuer-secret-value")
	verifier, _ := NewSigner("different-secret-value")
	tok, _ := issuer.Sign("d", "u", time.Minute)
	if _, err := verifier.Verify(tok); err != ErrSignature {
		t.Errorf("Expected ErrSignature with mismatched key, got %v", err)
	}
}

func TestEmptySecretRejected(t *testing.T) {
	if _, err := NewSigner(""); err != ErrInvalidKey {
		t.Errorf("Expected ErrInvalidKey for empty secret, got %v", err)
	}
}