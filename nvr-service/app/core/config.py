"""
Application configuration loaded from environment variables.

Uses pydantic-settings for type-safe configuration with validation.
All sensitive values default to empty and must be provided at runtime.
"""

from pydantic_settings import BaseSettings
from pydantic import field_validator
from functools import lru_cache


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    # Database
    DATABASE_URL: str = "postgresql+asyncpg://camguid:camguid@localhost:5432/camguid"

    # Encryption — Fernet key (base64-encoded, 32-byte key).
    # Generate with: python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"
    ENCRYPTION_KEY: str = ""

    # Stream access signing — used to create HMAC tokens for stream URLs
    STREAM_SIGNING_SECRET: str = "change-me-in-production"

    # Stream token TTL in seconds (default 5 minutes)
    STREAM_TOKEN_TTL: int = 300

    # FFmpeg / FFprobe timeout for RTSP validation (seconds)
    FFMPEG_TIMEOUT: int = 10

    # ONVIF support toggle
    ONVIF_ENABLED: bool = False

    # Server
    HOST: str = "0.0.0.0"
    PORT: int = 8090

    # Logging
    LOG_LEVEL: str = "info"

    model_config = {
        "env_prefix": "NVR_",
        "case_sensitive": False,
        "env_file": ".env",
        "env_file_encoding": "utf-8",
    }

    @field_validator("ENCRYPTION_KEY")
    @classmethod
    def validate_encryption_key(cls, v: str) -> str:
        if not v or _is_placeholder(v):
            raise ValueError(
                "NVR_ENCRYPTION_KEY is required and must not be a placeholder. "
                "Generate one with: "
                'python -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"'
            )
        # Fernet keys are base64-encoded and exactly 44 characters
        if len(v) != 44:
            raise ValueError(
                f"NVR_ENCRYPTION_KEY must be a valid Fernet key (44 characters base64). Got {len(v)} characters."
            )
        return v

    @field_validator("STREAM_SIGNING_SECRET")
    @classmethod
    def validate_signing_secret(cls, v: str) -> str:
        if not v or _is_placeholder(v):
            raise ValueError(
                "NVR_STREAM_SIGNING_SECRET is required and must not be a placeholder. "
                "Run scripts/generate-secrets.sh from the repository root."
            )
        return v

    @field_validator("DATABASE_URL")
    @classmethod
    def validate_database_url(cls, v: str) -> str:
        if not v:
            raise ValueError("NVR_DATABASE_URL is required.")
        return v


def _is_placeholder(v: str) -> bool:
    """True when a configured value is empty or a known example placeholder."""
    lowered = v.strip().lower()
    if not lowered:
        return True
    for prefix in ("change_me", "change-me", "ch4ng3me"):
        if lowered.startswith(prefix):
            return True
    return lowered in {
        "changeme-in-production",
        "change-me-in-production",
        "change-me-to-a-long-random-string",
        "camguid-nvr-stream-signing-secret-2026",
    }


@lru_cache()
def get_settings() -> Settings:
    """Cached settings singleton. Call get_settings.cache_clear() in tests."""
    return Settings()
