// Package streamtoken signs and verifies short-lived HMAC-SHA256 stream
// access tokens. Tokens carry the target device id, the requesting user id,
// and an expiration timestamp. Both the API (issuer) and the relay (verifier)
// share the same secret (CAMGUID_STREAM_SIGNING_SECRET) so verification is
// purely local — the relay never calls back into the API or nvr-service.
//
// The token is a url-safe base64 JSON payload "payload.signature" (hex HMAC),
// intentionally dependency-light and identical in spirit to the scheme used by
// nvr-service/app/core/security.py.
package streamtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrMalformed  = errors.New("malformed stream token")
	ErrSignature  = errors.New("stream token signature mismatch")
	ErrExpired    = errors.New("stream token has expired")
	ErrInvalidKey = errors.New("stream signing secret is empty")
)

// Payload is the token body.
type Payload struct {
	DeviceID string `json:"did"`
	UserID   string `json:"uid,omitempty"`
	Exp      int64  `json:"exp"` // unix seconds
}

type Signer struct {
	secret []byte
}

// NewSigner returns a Signer for the given secret. Empty secrets are rejected.
func NewSigner(secret string) (*Signer, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrInvalidKey
	}
	return &Signer{secret: []byte(secret)}, nil
}

// Sign mints a token for the given device that expires in ttl seconds.
func (s *Signer) Sign(deviceID, userID string, ttl time.Duration) (string, error) {
	payload := Payload{
		DeviceID: deviceID,
		UserID:   userID,
		Exp:      time.Now().Add(ttl).Unix(),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded + "." + s.signature(raw), nil
}

// Verify parses and validates a token, returning its decoded payload.
func (s *Signer) Verify(token string) (*Payload, error) {
	payload, _, err := s.parseAndVerify(token)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (s *Signer) parseAndVerify(token string) (*Payload, []byte, error) {
	parts := strings.SplitN(strings.TrimSpace(token), ".", 2)
	if len(parts) != 2 {
		return nil, nil, ErrMalformed
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, ErrMalformed
	}

	if !hmac.Equal([]byte(parts[1]), []byte(s.signature(raw))) {
		return nil, nil, ErrSignature
	}

	var payload Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, ErrMalformed
	}
	if time.Now().Unix() > payload.Exp {
		return nil, nil, ErrExpired
	}
	return &payload, raw, nil
}

func (s *Signer) signature(raw []byte) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}