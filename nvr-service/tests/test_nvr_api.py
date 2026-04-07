"""
Integration tests for the NVR API endpoints.

Uses FastAPI's TestClient with a mocked database to test the full
request/response cycle without requiring a live PostgreSQL instance.
"""

import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi.testclient import TestClient

from app.main import create_app


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _make_mock_nvr(**overrides):
    """Create a mock NVRDevice object."""
    defaults = {
        "id": uuid.uuid4(),
        "name": "Test NVR",
        "ip_address": "192.168.1.100",
        "rtsp_port": 554,
        "onvif_port": 80,
        "username": "admin",
        "password_encrypted": "encrypted_placeholder",
        "vendor": "hikvision",
        "channel_count": 4,
        "created_at": "2026-04-07T00:00:00+00:00",
        "updated_at": "2026-04-07T00:00:00+00:00",
        "channels": [],
    }
    defaults.update(overrides)

    mock = MagicMock()
    for key, value in defaults.items():
        setattr(mock, key, value)
    return mock


def _make_mock_channel(**overrides):
    """Create a mock NVRChannel object."""
    defaults = {
        "id": uuid.uuid4(),
        "nvr_id": uuid.uuid4(),
        "channel_number": 1,
        "name": "Test NVR - CH1",
        "stream_main_url": "rtsp://test:test@192.168.1.100:554/stream1",
        "stream_sub_url": "rtsp://test:test@192.168.1.100:554/stream1/sub",
        "enabled": True,
    }
    defaults.update(overrides)

    mock = MagicMock()
    for key, value in defaults.items():
        setattr(mock, key, value)
    return mock


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


class TestHealthEndpoint:
    """Tests for the /health endpoint."""

    def test_health_check(self):
        """Health check should return 200 with service info."""
        # Patch init_db to avoid DB connection
        with patch("app.main.init_db", new_callable=AsyncMock):
            app = create_app()
            client = TestClient(app)
            response = client.get("/health")

        assert response.status_code == 200
        data = response.json()
        assert data["status"] == "ok"
        assert data["service"] == "nvr-service"


class TestNVRCreateEndpoint:
    """Tests for POST /api/v1/nvr."""

    def test_create_nvr_success(self):
        """Should create an NVR and return 201."""
        mock_nvr = _make_mock_nvr()

        async def mock_get_db():
            yield AsyncMock()

        with (
            patch("app.main.init_db", new_callable=AsyncMock),
            patch("app.api.nvr_routes.nvr_service") as mock_svc,
            patch("app.core.database.get_db", mock_get_db),
        ):
            mock_svc.create_nvr = AsyncMock(return_value=mock_nvr)
            app = create_app()
            client = TestClient(app)

            response = client.post(
                "/api/v1/nvr",
                json={
                    "name": "Test NVR",
                    "ip_address": "192.168.1.100",
                    "vendor": "hikvision",
                    "username": "admin",
                    "password": "admin123",
                    "channel_count": 4,
                },
            )

        assert response.status_code == 201
        data = response.json()
        assert data["name"] == "Test NVR"
        assert data["vendor"] == "hikvision"
        # Password should NOT be in the response
        assert "password" not in data
        assert "password_encrypted" not in data

    def test_create_nvr_missing_fields(self):
        """Should return 422 for missing required fields."""
        with patch("app.main.init_db", new_callable=AsyncMock):
            app = create_app()
            client = TestClient(app)

            response = client.post(
                "/api/v1/nvr",
                json={"name": "Incomplete"},
            )

        assert response.status_code == 422


class TestNVRListEndpoint:
    """Tests for GET /api/v1/nvr."""

    def test_list_nvrs_empty(self):
        """Should return empty list when no NVRs exist."""
        async def mock_get_db():
            yield AsyncMock()

        with (
            patch("app.main.init_db", new_callable=AsyncMock),
            patch("app.api.nvr_routes.nvr_service") as mock_svc,
            patch("app.core.database.get_db", mock_get_db),
        ):
            mock_svc.list_nvrs = AsyncMock(return_value=[])
            app = create_app()
            client = TestClient(app)

            response = client.get("/api/v1/nvr")

        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 0
        assert data["data"] == []

    def test_list_nvrs_with_data(self):
        """Should return NVR list with proper structure."""
        mock_nvrs = [_make_mock_nvr(name="NVR-1"), _make_mock_nvr(name="NVR-2")]

        async def mock_get_db():
            yield AsyncMock()

        with (
            patch("app.main.init_db", new_callable=AsyncMock),
            patch("app.api.nvr_routes.nvr_service") as mock_svc,
            patch("app.core.database.get_db", mock_get_db),
        ):
            mock_svc.list_nvrs = AsyncMock(return_value=mock_nvrs)
            app = create_app()
            client = TestClient(app)

            response = client.get("/api/v1/nvr")

        assert response.status_code == 200
        data = response.json()
        assert data["total"] == 2


class TestStreamTokenEndpoint:
    """Tests for GET /api/v1/stream/{channel_id}."""

    def test_get_stream_token(self):
        """Should return a signed stream access token."""
        channel_id = uuid.uuid4()
        mock_channel = _make_mock_channel(id=channel_id)
        mock_channel.nvr = _make_mock_nvr()

        async def mock_get_db():
            yield AsyncMock()

        with (
            patch("app.main.init_db", new_callable=AsyncMock),
            patch("app.api.stream_routes.nvr_service") as mock_svc,
            patch("app.core.database.get_db", mock_get_db),
        ):
            mock_svc.get_channel = AsyncMock(return_value=mock_channel)
            app = create_app()
            client = TestClient(app)

            response = client.get(f"/api/v1/stream/{channel_id}?type=main")

        assert response.status_code == 200
        data = response.json()
        assert "token" in data
        assert data["stream_type"] == "main"
        assert data["expires_in"] > 0

    def test_invalid_stream_type(self):
        """Should reject invalid stream type."""
        channel_id = uuid.uuid4()

        with patch("app.main.init_db", new_callable=AsyncMock):
            app = create_app()
            client = TestClient(app)

            response = client.get(f"/api/v1/stream/{channel_id}?type=invalid")

        assert response.status_code == 422
