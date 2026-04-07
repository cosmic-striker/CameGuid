"""
Tests for the security module: Fernet encryption and stream token signing.
"""

import time

import pytest

from app.core.security import (
    create_stream_token,
    decrypt_password,
    encrypt_password,
    verify_stream_token,
)


class TestPasswordEncryption:
    """Tests for Fernet password encrypt/decrypt."""

    def test_encrypt_decrypt_roundtrip(self):
        """Encrypted passwords should decrypt back to the original."""
        original = "SuperSecretP@ss123!"
        encrypted = encrypt_password(original)

        # Ciphertext should not equal plaintext
        assert encrypted != original

        # Decrypt should return original
        decrypted = decrypt_password(encrypted)
        assert decrypted == original

    def test_encrypt_produces_different_ciphertext(self):
        """Each encryption should produce a different ciphertext (random IV)."""
        password = "test-password"
        enc1 = encrypt_password(password)
        enc2 = encrypt_password(password)

        # Fernet uses a random IV, so ciphertexts should differ
        assert enc1 != enc2

        # Both should decrypt to the same value
        assert decrypt_password(enc1) == password
        assert decrypt_password(enc2) == password

    def test_encrypt_empty_password_raises(self):
        """Cannot encrypt an empty string."""
        with pytest.raises(ValueError, match="empty"):
            encrypt_password("")

    def test_decrypt_empty_ciphertext_raises(self):
        """Cannot decrypt an empty string."""
        with pytest.raises(ValueError, match="empty"):
            decrypt_password("")

    def test_decrypt_invalid_ciphertext_raises(self):
        """Decrypting garbage should raise ValueError."""
        with pytest.raises(ValueError, match="decrypt"):
            decrypt_password("this-is-not-valid-fernet-ciphertext")

    def test_encrypt_special_characters(self):
        """Passwords with special characters should roundtrip correctly."""
        special = "p@$$w0rd!#%^&*()_+-=[]{}|;':\",./<>?`~"
        encrypted = encrypt_password(special)
        assert decrypt_password(encrypted) == special

    def test_encrypt_unicode(self):
        """Unicode passwords should roundtrip correctly."""
        unicode_pw = "密码テスト🔐"
        encrypted = encrypt_password(unicode_pw)
        assert decrypt_password(encrypted) == unicode_pw

    def test_encrypt_long_password(self):
        """Very long passwords should work."""
        long_pw = "a" * 10000
        encrypted = encrypt_password(long_pw)
        assert decrypt_password(encrypted) == long_pw


class TestStreamTokens:
    """Tests for HMAC-signed stream access tokens."""

    def test_create_and_verify_token(self):
        """Created tokens should verify successfully."""
        token = create_stream_token(
            channel_id="550e8400-e29b-41d4-a716-446655440000",
            stream_type="main",
            ttl=60,
        )

        # Token should be a string with two parts
        assert "." in token

        payload = verify_stream_token(token)
        assert payload["cid"] == "550e8400-e29b-41d4-a716-446655440000"
        assert payload["type"] == "main"
        assert payload["exp"] > time.time()

    def test_verify_expired_token_raises(self):
        """Expired tokens should be rejected."""
        token = create_stream_token(
            channel_id="test-channel",
            stream_type="sub",
            ttl=0,  # Expires immediately
        )

        # Wait a moment to ensure expiration
        time.sleep(0.1)

        with pytest.raises(ValueError, match="expired"):
            verify_stream_token(token)

    def test_verify_tampered_token_raises(self):
        """Tokens with altered payloads should be rejected."""
        token = create_stream_token(
            channel_id="test-channel",
            stream_type="main",
            ttl=60,
        )

        # Tamper with the payload part
        parts = token.split(".", 1)
        tampered = "dGFtcGVyZWQ=" + "." + parts[1]

        with pytest.raises(ValueError, match="(signature|decode)"):
            verify_stream_token(tampered)

    def test_verify_malformed_token_raises(self):
        """Malformed tokens should be rejected."""
        with pytest.raises(ValueError, match="Malformed"):
            verify_stream_token("not-a-valid-token-at-all")

    def test_sub_stream_type(self):
        """Sub stream type should be preserved in token."""
        token = create_stream_token(
            channel_id="test-channel",
            stream_type="sub",
            ttl=60,
        )
        payload = verify_stream_token(token)
        assert payload["type"] == "sub"
