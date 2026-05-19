package discord

import (
	"crypto/ed25519"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestVerifySignatureValid(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	body := []byte(`{"type":1}`)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))
	if err := VerifySignature(hex.EncodeToString(publicKey), timestamp, body, hex.EncodeToString(signature), now, 5*time.Minute); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
}

func TestVerifySignatureRejectsInvalidSignature(t *testing.T) {
	publicKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	badSignature := make([]byte, ed25519.SignatureSize)
	if err := VerifySignature(hex.EncodeToString(publicKey), "1000", []byte(`{"type":1}`), hex.EncodeToString(badSignature), now, 5*time.Minute); err == nil {
		t.Fatal("invalid signature accepted")
	}
}

func TestVerifySignatureRejectsReplayWindow(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"type":1}`)
	timestamp := "1000"
	signature := ed25519.Sign(privateKey, append([]byte(timestamp), body...))
	if err := VerifySignature(hex.EncodeToString(publicKey), timestamp, body, hex.EncodeToString(signature), time.Unix(2000, 0), 5*time.Minute); err == nil {
		t.Fatal("expired timestamp accepted")
	}
}
