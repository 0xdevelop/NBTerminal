package security

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/george012/gtbox/gtbox_encryption"
)

const encryptedEnvelopeVersion = "v1"

func EncryptPayloadGT(plaintext, key string) (string, error) {
	if plaintext == "" || key == "" {
		return "", errors.New("encrypt payload requires plaintext and key")
	}
	ciphertext := gtbox_encryption.GTEnc(plaintext, key)
	if ciphertext == "" {
		return "", errors.New("GTEnc failed")
	}
	encoded := base64.RawURLEncoding.EncodeToString([]byte(ciphertext))
	mac := payloadMAC(encryptedEnvelopeVersion, encoded, key)
	return encryptedEnvelopeVersion + "." + encoded + "." + base64.RawURLEncoding.EncodeToString(mac), nil
}

func DecryptPayloadGT(envelope, key string) (string, error) {
	if envelope == "" || key == "" {
		return "", errors.New("decrypt payload requires envelope and key")
	}
	parts := strings.Split(envelope, ".")
	if len(parts) != 3 || parts[0] != encryptedEnvelopeVersion {
		return "", errors.New("unsupported encrypted payload envelope")
	}
	providedMAC, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(providedMAC, payloadMAC(parts[0], parts[1], key)) {
		return "", errors.New("encrypted payload integrity check failed")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(ciphertext) == 0 {
		return "", errors.New("decode encrypted payload")
	}
	plaintext := gtbox_encryption.GTDec(string(ciphertext), key)
	if plaintext == "" {
		return "", errors.New("GTDec failed")
	}
	return plaintext, nil
}

func payloadMAC(version, encodedCiphertext, key string) []byte {
	derived := sha256.Sum256([]byte("nbterminal-gt-envelope-mac-v1\x00" + key))
	mac := hmac.New(sha256.New, derived[:])
	_, _ = mac.Write([]byte(version))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(encodedCiphertext))
	return mac.Sum(nil)
}
