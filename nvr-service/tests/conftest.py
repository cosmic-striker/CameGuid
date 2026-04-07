"""
Pytest configuration and shared fixtures.

Provides a test-scoped settings override with a known Fernet key
so that tests don't require real environment variables.
"""

import os
import pytest

from cryptography.fernet import Fernet

# Generate a deterministic test key
TEST_FERNET_KEY = Fernet.generate_key().decode()
TEST_SIGNING_SECRET = "test-signing-secret-do-not-use-in-production"


@pytest.fixture(autouse=True)
def set_test_env(monkeypatch):
    """Set up test environment variables before each test."""
    monkeypatch.setenv("NVR_ENCRYPTION_KEY", TEST_FERNET_KEY)
    monkeypatch.setenv("NVR_STREAM_SIGNING_SECRET", TEST_SIGNING_SECRET)
    monkeypatch.setenv("NVR_DATABASE_URL", "postgresql+asyncpg://test:test@localhost:5432/test")
    monkeypatch.setenv("NVR_ONVIF_ENABLED", "false")

    # Clear the cached settings so each test gets fresh config
    from app.core.config import get_settings
    get_settings.cache_clear()

    yield

    get_settings.cache_clear()
