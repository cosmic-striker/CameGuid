"""
Tests for NVR channel auto-discovery: vendor response parsers, the Digest
auth helper, and the discovery service orchestration (including the
channel_count fallback).
"""

import hashlib
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.core.exceptions import UnsupportedVendorError
from app.models.channel import NVRChannel
from app.services.discovery_service import discover_nvr_channels
from app.vendors.dahua import parse_dahua_channels
from app.vendors.hikvision import parse_hikvision_channels
from app.vendors.http_client import (
    build_digest_auth_header,
    parse_digest_challenge,
)

# Sample Hikvision ISAPI InputProxyChannelList response
HIKVISION_XML = """<?xml version="1.0" encoding="UTF-8"?>
<InputProxyChannelList version="2.0" xmlns="http://www.hikvision.com/ver20/XMLSchema">
    <InputProxyChannel>
        <id>1</id>
        <name>Front Gate</name>
        <inputPort>1</inputPort>
        <resideStatus>inplace</resideStatus>
        <channelType>analog</channelType>
        <enabled>true</enabled>
        <online>true</online>
    </InputProxyChannel>
    <InputProxyChannel>
        <id>2</id>
        <name>Back Yard</name>
        <inputPort>2</inputPort>
        <resideStatus>inplace</resideStatus>
        <channelType>analog</channelType>
        <enabled>true</enabled>
        <online>false</online>
    </InputProxyChannel>
    <InputProxyChannel>
        <id>4</id>
        <name>Warehouse</name>
        <inputPort>4</inputPort>
        <resideStatus>inplace</resideStatus>
        <channelType>ip</channelType>
        <enabled>false</enabled>
        <online>false</online>
    </InputProxyChannel>
</InputProxyChannelList>
"""

# Sample Dahua configManager ChannelTitle response
DAHUA_CGI = """ChannelTitle.1=Front Gate
ChannelTitle.2=
ChannelTitle.3=Parking Lot
ChannelTitle.4=Loading Dock
ChannelTitle.5=
ChannelTitle.6=Back Gate
"""


# ---------------------------------------------------------------------------
# Hikvision parser
# ---------------------------------------------------------------------------


class TestHikvisionParsing:
    def test_parses_connected_channels(self):
        channels = parse_hikvision_channels(HIKVISION_XML)
        assert len(channels) == 3
        assert [c.channel_number for c in channels] == [1, 2, 4]
        assert channels[0].name == "Front Gate"
        assert channels[0].enabled is True
        assert channels[0].online is True

    def test_honours_enabled_and_online_flags(self):
        channels = parse_hikvision_channels(HIKVISION_XML)
        assert channels[1].online is False
        assert channels[2].enabled is False

    def test_invalid_xml_returns_empty(self):
        assert parse_hikvision_channels("this is not xml") == []

    def test_empty_response_returns_empty(self):
        assert parse_hikvision_channels("") == []


# ---------------------------------------------------------------------------
# Dahua parser
# ---------------------------------------------------------------------------


class TestDahuaParsing:
    def test_keeps_only_named_channels(self):
        channels = parse_dahua_channels(DAHUA_CGI)
        # Channels 1, 3, 4, 6 have titles; 2 and 5 are empty (no camera)
        assert [c.channel_number for c in channels] == [1, 3, 4, 6]
        assert channels[0].name == "Front Gate"

    def test_sorted_by_channel_number(self):
        channels = parse_dahua_channels(
            "ChannelTitle.4=Four\nChannelTitle.1=One\nChannelTitle.10=Ten"
        )
        assert [c.channel_number for c in channels] == [1, 4, 10]

    def test_all_empty_falls_back_to_full_list(self):
        channels = parse_dahua_channels("ChannelTitle.1=\nChannelTitle.2=")
        # Nothing named — return the parsed entries (caller falls back)
        assert len(channels) == 2
        assert channels[0].name is None

    def test_response_without_titles_returns_empty(self):
        assert parse_dahua_channels("SomeOtherKey=42") == []
        assert parse_dahua_channels("") == []


# ---------------------------------------------------------------------------
# Digest auth helper
# ---------------------------------------------------------------------------


class TestDigestAuth:
    def test_build_digest_header_is_deterministic(self):
        challenge = (
            'Digest realm="nvr-realm",nonce="MTIzNDU2Nzg5",qop="auth",'
            'opaque="abc",algorithm=MD5'
        )
        header = build_digest_auth_header(
            "GET",
            "/ISAPI/ContentMgmt/InputProxy/channels",
            "admin",
            "pass123",
            challenge,
            nc="00000001",
            cnonce="deadbeefdeadbeef",
        )
        assert header is not None

        # Verify the computed response against an independent implementation
        realm = "nvr-realm"
        nonce = "MTIzNDU2Nzg5"
        qop = "auth"
        nc = "00000001"
        cnonce = "deadbeefdeadbeef"
        method = "GET"
        uri = "/ISAPI/ContentMgmt/InputProxy/channels"

        ha1 = hashlib.md5(f"admin:{realm}:pass123".encode()).hexdigest()
        ha2 = hashlib.md5(f"{method}:{uri}".encode()).hexdigest()
        expected = hashlib.md5(
            f"{ha1}:{nonce}:{nc}:{cnonce}:{qop}:{ha2}".encode()
        ).hexdigest()

        assert f'response="{expected}"' in header

    def test_returns_none_for_non_digest_challenge(self):
        header = build_digest_auth_header(
            "GET", "/path", "admin", "pass", 'Basic realm="x"'
        )
        assert header is None

    def test_returns_none_when_missing_nonce(self):
        header = build_digest_auth_header(
            "GET", "/path", "admin", "pass", 'Digest realm="only-realm"'
        )
        assert header is None

    def test_parse_digest_challenge(self):
        params = parse_digest_challenge(
            'Digest realm="r",nonce="n1",qop="auth",opaque="op",algorithm=MD5'
        )
        assert params == {
            "realm": "r",
            "nonce": "n1",
            "qop": "auth",
            "opaque": "op",
            "algorithm": "MD5",
        }


# ---------------------------------------------------------------------------
# Discovery service orchestration
# ---------------------------------------------------------------------------

HIKVISION_URL = (
    "rtsp://admin:secret@192.168.1.50:554/Streaming/Channels/101"
)


def _make_mock_nvr(**overrides):
    defaults = {
        "id": uuid.uuid4(),
        "name": "Floor-1",
        "ip_address": "192.168.1.50",
        "rtsp_port": 554,
        "onvif_port": 80,
        "username": "admin",
        "password_encrypted": "encrypted",
        "vendor": "hikvision",
        "channel_count": 32,
    }
    defaults.update(overrides)
    mock = MagicMock()
    for key, value in defaults.items():
        setattr(mock, key, value)
    mock.discover_channels = AsyncMock()
    return mock


def _mock_db():
    db = AsyncMock()
    db.add = MagicMock()
    return db


class TestDiscoverNVRChannels:
    @pytest.mark.asyncio
    async def test_syncs_discovered_channels_to_db(self):
        nvr = _make_mock_nvr()
        from app.vendors.hikvision import DiscoveredChannel

        async def fake_discover(**kwargs):
            return [
                DiscoveredChannel(
                    channel_number=1, name="Front Gate", enabled=True, online=True
                ),
                DiscoveredChannel(
                    channel_number=2, name="Back Yard", enabled=False
                ),
            ]

        db = _mock_db()

        with (
            patch("app.services.discovery_service.nvr_service.get_nvr",
                  new_callable=AsyncMock, return_value=nvr),
            patch("app.services.discovery_service.decrypt_password",
                  return_value="secret"),
            patch("app.vendors.hikvision.HikvisionStrategy.discover_channels",
                  side_effect=fake_discover),
        ):
            channels = await discover_nvr_channels(db, nvr.id)

        assert len(channels) == 2
        assert isinstance(channels[0], NVRChannel)
        assert channels[0].channel_number == 1
        assert channels[0].name == "Front Gate"
        assert channels[1].name == "Back Yard"
        assert channels[1].enabled is False
        # channel_count synced to the real discovered count
        assert nvr.channel_count == 2
        assert db.add.call_count == 2

    @pytest.mark.asyncio
    async def test_falls_back_to_channel_count_when_no_channels(self):
        nvr = _make_mock_nvr()
        fallback_channels = [MagicMock(), MagicMock()]

        db = _mock_db()

        with (
            patch("app.services.discovery_service.nvr_service.get_nvr",
                  new_callable=AsyncMock, return_value=nvr),
            patch("app.services.discovery_service.decrypt_password",
                  return_value="secret"),
            patch("app.vendors.hikvision.HikvisionStrategy.discover_channels",
                  new_callable=AsyncMock, return_value=None),
            patch("app.services.discovery_service.nvr_service.generate_channels",
                  new_callable=AsyncMock, return_value=fallback_channels) as gen,
        ):
            result = await discover_nvr_channels(db, nvr.id)

        assert result == fallback_channels
        gen.assert_awaited_once_with(db, nvr.id)

    @pytest.mark.asyncio
    async def test_falls_back_for_generic_vendor_without_http_call(self):
        nvr = _make_mock_nvr(vendor="generic")

        db = _mock_db()

        with (
            patch("app.services.discovery_service.nvr_service.get_nvr",
                  new_callable=AsyncMock, return_value=nvr),
            patch("app.services.discovery_service.decrypt_password",
                  return_value="secret"),
            patch("app.services.discovery_service.nvr_service.generate_channels",
                  new_callable=AsyncMock, return_value=[]) as gen,
            patch("app.vendors.generic.GenericStrategy.discover_channels",
                  new_callable=AsyncMock) as generic_discover,
        ):
            await discover_nvr_channels(db, nvr.id)

        # Generic strategy must never be invoked for HTTP discovery
        generic_discover.assert_not_awaited()
        gen.assert_awaited_once_with(db, nvr.id)

    @pytest.mark.asyncio
    async def test_raises_for_unsupported_vendor(self):
        nvr = _make_mock_nvr(vendor="unknown-brand")

        db = _mock_db()

        with (
            patch("app.services.discovery_service.nvr_service.get_nvr",
                  new_callable=AsyncMock, return_value=nvr),
            patch("app.services.discovery_service.decrypt_password",
                  return_value="secret"),
            patch("app.vendors.get_vendor_strategy", side_effect=KeyError("x")),
        ):
            with pytest.raises(UnsupportedVendorError):
                await discover_nvr_channels(db, nvr.id)


# ---------------------------------------------------------------------------
# API-level route test
# ---------------------------------------------------------------------------


class TestDiscoverEndpoint:
    def test_discover_endpoint_success(self):
        from fastapi.testclient import TestClient

        from app.main import create_app

        channel = NVRChannel(
            id=uuid.uuid4(),
            nvr_id=uuid.uuid4(),
            channel_number=3,
            name="Parking Lot",
            stream_main_url=HIKVISION_URL,
            stream_sub_url=HIKVISION_URL.replace("101", "102"),
            enabled=True,
        )

        async def mock_get_db():
            yield AsyncMock()

        with (
            patch("app.main.init_db", new_callable=AsyncMock),
            patch("app.api.nvr_routes.discovery_service") as mock_svc,
            patch("app.core.database.get_db", mock_get_db),
        ):
            mock_svc.discover_nvr_channels = AsyncMock(return_value=[channel])
            app = create_app()
            client = TestClient(app)

            response = client.post(f"/api/v1/nvr/{channel.nvr_id}/discover")

        assert response.status_code == 200
        data = response.json()
        assert data["channels_created"] == 1
        assert data["channels"][0]["name"] == "Parking Lot"
        assert data["channels"][0]["channel_number"] == 3
        # Stream URLs must never leak in responses
        assert "stream_main_url" not in data["channels"][0]