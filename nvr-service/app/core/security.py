"""
Security utilities: Fernet encryption for NVR passwords and
HMAC-signed tokens for secure stream access.

Passwords are encrypted at rest using Fernet symmetric encryption.
Stream tokens are short-lived HMAC-SHA256 signed JWTs that grant
access to a specific channel stream without exposing raw RTSP URLs.
"""

import hashlib
import hmac
import json
import time
from base64 import urlsafe_b64decode, urlsafe_b64encode

from cryptography.fernet import Fernet, InvalidToken

from app.core.config import get_settings


# ---------------------------------------------------------------------------
# Password Encryption (Fernet)
# ---------------------------------------------------------------------------


def _get_fernet() -> Fernet:
    """Create a Fernet cipher from the configured encryption key."""
    settings = get_settings()
    return Fernet(settings.ENCRYPTION_KEY.encode())


def encrypt_password(plaintext: str) -> str:
    """
    Encrypt a plaintext password using Fernet.

    Returns a URL-safe base64-encoded ciphertext string suitable for
    database storage.
    """
    if not plaintext:
        raise ValueError("Cannot encrypt an empty password.")
    fernet = _get_fernet()
    token = fernet.encrypt(plaintext.encode("utf-8"))
    return token.decode("utf-8")


def decrypt_password(ciphertext: str) -> str:
    """
    Decrypt a Fernet-encrypted password.

    Raises ValueError if the ciphertext is invalid or the key is wrong.
    """
    if not ciphertext:
        raise ValueError("Cannot decrypt an empty ciphertext.")
    fernet = _get_fernet()
    try:
        plaintext = fernet.decrypt(ciphertext.encode("utf-8"))
        return plaintext.decode("utf-8")
    except InvalidToken as exc:
        raise ValueError(
            "Failed to decrypt password — the encryption key may have changed "
            "or the ciphertext is corrupted."
        ) from exc


# ---------------------------------------------------------------------------
# Stream Access Tokens (HMAC-SHA256 signed)
# ---------------------------------------------------------------------------


def create_stream_token(
    channel_id: str,
    stream_type: str = "main",
    ttl: int | None = None,
) -> str:
    """
    Create a signed stream access token.

    The token encodes the channel_id, stream_type, and an expiration
    timestamp. It is NOT a full JWT — it is a lightweight HMAC-signed
    JSON payload to avoid adding a JWT library dependency.

    Args:
        channel_id: UUID of the channel to grant access to.
        stream_type: "main" or "sub".
        ttl: Time-to-live in seconds. Defaults to settings.STREAM_TOKEN_TTL.

    Returns:
        A URL-safe base64-encoded signed token string.
    """
    settings = get_settings()
    if ttl is None:
        ttl = settings.STREAM_TOKEN_TTL

    payload = {
        "cid": str(channel_id),
        "type": stream_type,
        "exp": int(time.time()) + ttl,
    }

    payload_bytes = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    payload_b64 = urlsafe_b64encode(payload_bytes).decode("utf-8")

    signature = hmac.new(
        settings.STREAM_SIGNING_SECRET.encode("utf-8"),
        payload_bytes,
        hashlib.sha256,
    ).hexdigest()

    return f"{payload_b64}.{signature}"


def verify_stream_token(token: str) -> dict:
    """
    Verify and decode a stream access token.

    Raises ValueError if the token is invalid, tampered with, or expired.

    Returns:
        Decoded payload dict with keys: cid, type, exp.
    """
    settings = get_settings()

    parts = token.split(".", 1)
    if len(parts) != 2:
        raise ValueError("Malformed stream token.")

    payload_b64, provided_signature = parts

    try:
        payload_bytes = urlsafe_b64decode(payload_b64)
        payload = json.loads(payload_bytes)
    except Exception as exc:
        raise ValueError("Failed to decode stream token payload.") from exc

    # Verify HMAC signature
    expected_signature = hmac.new(
        settings.STREAM_SIGNING_SECRET.encode("utf-8"),
        payload_bytes,
        hashlib.sha256,
    ).hexdigest()

    if not hmac.compare_digest(provided_signature, expected_signature):
        raise ValueError("Stream token signature verification failed.")

    # Check expiration
    if time.time() > payload.get("exp", 0):
        raise ValueError("Stream token has expired.")

    return payload
