package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAgentTokenIsShortLivedAndSignedWithGatewaySecret(t *testing.T) {
	gateway := &gateway{secret: []byte("test-secret-that-is-long-enough")}
	raw, err := gateway.agentToken()
	if err != nil {
		t.Fatalf("agentToken: %v", err)
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		t.Fatalf("token parts = %d, want 3", len(parts))
	}
	mac := hmac.New(sha256.New, gateway.secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(parts[2]), []byte(wantSignature)) {
		t.Fatal("token signature does not verify")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	claims := &gatewayClaims{}
	if err := json.Unmarshal(payload, claims); err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	if claims.UserID != "video-studio" || claims.Username != "video-studio" || claims.Provider != "gateway" {
		t.Fatalf("claims = %#v", claims)
	}
	untilExpiry := time.Until(time.Unix(claims.ExpiresAt, 0))
	if untilExpiry <= 0 || untilExpiry > 16*time.Minute {
		t.Fatalf("unexpected expiration: %d", claims.ExpiresAt)
	}
}
