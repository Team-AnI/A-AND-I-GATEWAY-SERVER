package discord

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"
)

func VerifySignature(publicKeyHex, timestamp string, body []byte, signatureHex string, now time.Time, replayWindow time.Duration) error {
	publicKey, err := hex.DecodeString(publicKeyHex)
	if err != nil || len(publicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid discord public key")
	}
	signature, err := hex.DecodeString(signatureHex)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("invalid discord signature")
	}
	signedAt, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid discord timestamp")
	}
	requestTime := time.Unix(signedAt, 0)
	if now.Sub(requestTime) > replayWindow || requestTime.Sub(now) > replayWindow {
		return fmt.Errorf("discord timestamp outside replay window")
	}
	message := append([]byte(timestamp), body...)
	if !ed25519.Verify(publicKey, message, signature) {
		return fmt.Errorf("discord signature verification failed")
	}
	return nil
}
