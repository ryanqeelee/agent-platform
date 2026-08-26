package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

const oidcStateMaxAge = 10 * time.Minute

// OIDCStatePayload is the signed OIDC authorization state carried in the
// redirect URL and validated on callback.
type OIDCStatePayload struct {
	Nonce       string `json:"nonce"`
	RedirectURI string `json:"redirect_uri,omitempty"`
	ReturnTo    string `json:"return_to,omitempty"`
	IssuedAt    int64  `json:"iat"`
}

var (
	oidcStateSecretOnce sync.Once
	oidcStateSecret     string
)

func oidcStateSigningKey() string {
	oidcStateSecretOnce.Do(func() {
		if envSecret := strings.TrimSpace(os.Getenv("JWT_SECRET")); envSecret != "" {
			oidcStateSecret = envSecret
			return
		}
		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			panic(fmt.Sprintf("failed to generate OIDC state signing key: %v", err))
		}
		oidcStateSecret = base64.StdEncoding.EncodeToString(randomBytes)
	})
	return oidcStateSecret
}

// SignOIDCState returns a tamper-evident state token: base64url(payload).base64url(hmac).
func SignOIDCState(payload *OIDCStatePayload) (string, error) {
	if payload == nil {
		return "", errors.New("oidc state payload is required")
	}
	if strings.TrimSpace(payload.Nonce) == "" {
		return "", errors.New("oidc state nonce is required")
	}
	if strings.TrimSpace(payload.RedirectURI) == "" {
		return "", errors.New("oidc state redirect_uri is required")
	}
	if _, err := ValidateOIDCReturnTo(payload.ReturnTo); err != nil {
		return "", err
	}
	if payload.IssuedAt == 0 {
		payload.IssuedAt = time.Now().Unix()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal oidc state: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(oidcStateSigningKey()))
	mac.Write(raw)
	sig := mac.Sum(nil)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

// VerifyOIDCState validates the HMAC and freshness of a state token.
func VerifyOIDCState(raw string) (*OIDCStatePayload, error) {
	raw = strings.TrimSpace(raw)
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return nil, errors.New("invalid oidc state format")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode oidc state payload: %w", err)
	}
	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode oidc state signature: %w", err)
	}
	mac := hmac.New(sha256.New, []byte(oidcStateSigningKey()))
	mac.Write(payloadBytes)
	if !hmac.Equal(mac.Sum(nil), sigBytes) {
		return nil, errors.New("oidc state signature mismatch")
	}
	var payload OIDCStatePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal oidc state: %w", err)
	}
	if strings.TrimSpace(payload.RedirectURI) == "" {
		return nil, errors.New("state.redirect_uri is required")
	}
	if _, err := ValidateOIDCReturnTo(payload.ReturnTo); err != nil {
		return nil, err
	}
	if payload.IssuedAt == 0 {
		return nil, errors.New("state.iat is required")
	}
	issuedAt := time.Unix(payload.IssuedAt, 0)
	if time.Since(issuedAt) > oidcStateMaxAge || time.Until(issuedAt) > time.Minute {
		return nil, errors.New("oidc state expired or invalid timestamp")
	}
	return &payload, nil
}

// ValidateOIDCReturnTo accepts only a same-origin product route for an OIDC
// round-trip. The SPA performs the final route-resolution check after login;
// this boundary prevents state from carrying an external or malformed target.
func ValidateOIDCReturnTo(returnTo string) (string, error) {
	if returnTo == "" {
		return "", nil
	}
	path := strings.SplitN(returnTo, "?", 2)[0]
	if strings.TrimSpace(returnTo) != returnTo ||
		!strings.HasPrefix(returnTo, "/platform/") ||
		strings.Contains(path, "//") ||
		strings.Contains(returnTo, "\\") ||
		strings.ContainsAny(returnTo, "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f\x7f") {
		return "", errors.New("oidc state return_to must be a safe /platform/ path")
	}
	return returnTo, nil
}
