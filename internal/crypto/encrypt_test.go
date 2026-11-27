package crypto

import (
	"testing"
)

const testSecret = "test-secret-key-for-unit-tests"

func TestDeriveKey(t *testing.T) {
	key, err := DeriveKey(testSecret)
	if err != nil {
		t.Fatalf("DeriveKey: %v", err)
	}
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}

	// Deterministic: same secret → same key.
	key2, _ := DeriveKey(testSecret)
	if string(key) != string(key2) {
		t.Fatal("DeriveKey not deterministic")
	}
}

func TestDeriveKeyEmptySecret(t *testing.T) {
	_, err := DeriveKey("")
	if err == nil {
		t.Fatal("expected error for empty secret")
	}
}

func TestRoundTrip(t *testing.T) {
	key, _ := DeriveKey(testSecret)

	original := "my-super-secret-password"
	encrypted, err := EncryptField(original, key)
	if err != nil {
		t.Fatalf("EncryptField: %v", err)
	}

	if encrypted == original {
		t.Fatal("encrypted value should differ from plaintext")
	}

	decrypted, err := DecryptField(encrypted, key)
	if err != nil {
		t.Fatalf("DecryptField: %v", err)
	}

	if decrypted != original {
		t.Fatalf("round-trip failed: got %q, want %q", decrypted, original)
	}
}

func TestEmptyStringPassthrough(t *testing.T) {
	key, _ := DeriveKey(testSecret)

	encrypted, err := EncryptField("", key)
	if err != nil {
		t.Fatalf("EncryptField empty: %v", err)
	}
	if encrypted != "" {
		t.Fatalf("expected empty string, got %q", encrypted)
	}

	decrypted, err := DecryptField("", key)
	if err != nil {
		t.Fatalf("DecryptField empty: %v", err)
	}
	if decrypted != "" {
		t.Fatalf("expected empty string, got %q", decrypted)
	}
}

func TestPlaintextFallback(t *testing.T) {
	key, _ := DeriveKey(testSecret)

	// Simulate a legacy plaintext value stored before encryption was enabled.
	plaintext := "old-password-in-db"
	decrypted, err := DecryptField(plaintext, key)
	if err != nil {
		t.Fatalf("DecryptField plaintext fallback: %v", err)
	}
	if decrypted != plaintext {
		t.Fatalf("expected %q, got %q", plaintext, decrypted)
	}
}

func TestWrongKeyReturnsError(t *testing.T) {
	key1, _ := DeriveKey("secret-one")
	key2, _ := DeriveKey("secret-two")

	encrypted, err := EncryptField("sensitive-data", key1)
	if err != nil {
		t.Fatalf("EncryptField: %v", err)
	}

	_, err = DecryptField(encrypted, key2)
	if err == nil {
		t.Fatal("expected error when decrypting with wrong key")
	}
}

func TestDifferentCiphertextsForSamePlaintext(t *testing.T) {
	key, _ := DeriveKey(testSecret)

	enc1, _ := EncryptField("same-value", key)
	enc2, _ := EncryptField("same-value", key)

	if enc1 == enc2 {
		t.Fatal("two encryptions of same plaintext should produce different ciphertext (random nonce)")
	}

	// Both should decrypt to the same value.
	dec1, _ := DecryptField(enc1, key)
	dec2, _ := DecryptField(enc2, key)
	if dec1 != dec2 {
		t.Fatal("both ciphertexts should decrypt to same plaintext")
	}
}

func TestEncryptedValueHasPrefix(t *testing.T) {
	key, _ := DeriveKey(testSecret)

	encrypted, _ := EncryptField("test", key)
	if encrypted[:7] != "enc:v1:" {
		t.Fatalf("expected enc:v1: prefix, got %q", encrypted[:7])
	}
}

func TestUnsupportedVersion(t *testing.T) {
	key, _ := DeriveKey(testSecret)

	_, err := DecryptField("enc:v99:invaliddata", key)
	if err == nil {
		t.Fatal("expected error for unsupported version")
	}
}
