package security

import (
	"strings"
	"testing"
)

func TestGTPayloadEnvelopeRoundTripAndTamperDetection(t *testing.T) {
	key := "synthetic-256-bit-purpose-key-for-test"
	envelope, err := EncryptPayloadGT(`{"secret":"synthetic-marker"}`, key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(envelope, "v1.") || strings.Contains(envelope, "synthetic-marker") {
		t.Fatalf("unexpected envelope shape: %q", envelope)
	}
	plaintext, err := DecryptPayloadGT(envelope, key)
	if err != nil || plaintext != `{"secret":"synthetic-marker"}` {
		t.Fatalf("plaintext=%q err=%v", plaintext, err)
	}
	for _, invalid := range []string{
		envelope[:len(envelope)-1] + "A",
		envelope[:len(envelope)/2],
		"v2." + strings.TrimPrefix(envelope, "v1."),
	} {
		if _, err := DecryptPayloadGT(invalid, key); err == nil {
			t.Fatalf("invalid envelope was accepted: %q", invalid)
		}
	}
	if _, err := DecryptPayloadGT(envelope, key+"-wrong"); err == nil {
		t.Fatal("wrong envelope key was accepted")
	}
}
