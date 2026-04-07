"""
Tests for the RTSP URL generation service.

These tests verify that each vendor strategy produces the correct
RTSP URL format. No network access is required.
"""

import pytest

from app.vendors.hikvision import HikvisionStrategy
from app.vendors.dahua import DahuaStrategy
from app.vendors.generic import GenericStrategy
from app.vendors import get_vendor_strategy, VENDOR_STRATEGIES


class TestHikvisionStrategy:
    """Tests for Hikvision RTSP URL generation."""

    def setup_method(self):
        self.strategy = HikvisionStrategy()

    def test_main_stream_url(self):
        url = self.strategy.build_main_stream_url(
            ip="192.168.1.100",
            port=554,
            username="admin",
            password="pass123",
            channel=1,
        )
        assert url == "rtsp://admin:pass123@192.168.1.100:554/Streaming/Channels/101"

    def test_sub_stream_url(self):
        url = self.strategy.build_sub_stream_url(
            ip="192.168.1.100",
            port=554,
            username="admin",
            password="pass123",
            channel=1,
        )
        assert url == "rtsp://admin:pass123@192.168.1.100:554/Streaming/Channels/102"

    def test_channel_numbering(self):
        """Channel numbers should be used directly in the URL."""
        url = self.strategy.build_main_stream_url(
            ip="10.0.0.1",
            port=554,
            username="user",
            password="pw",
            channel=16,
        )
        assert "/Streaming/Channels/1601" in url

    def test_special_chars_in_password(self):
        """Special characters in passwords should be URL-encoded."""
        url = self.strategy.build_main_stream_url(
            ip="192.168.1.1",
            port=554,
            username="admin",
            password="p@ss:word",
            channel=1,
        )
        assert "p%40ss%3Aword" in url

    def test_custom_port(self):
        """Non-default RTSP ports should be used."""
        url = self.strategy.build_main_stream_url(
            ip="192.168.1.1",
            port=8554,
            username="admin",
            password="pass",
            channel=1,
        )
        assert ":8554/" in url

    def test_build_url_dispatcher_main(self):
        """build_url should dispatch to main stream."""
        url = self.strategy.build_url(
            ip="192.168.1.1", port=554, username="a", password="b",
            channel=1, stream_type="main",
        )
        assert "01" in url  # Main stream suffix

    def test_build_url_dispatcher_sub(self):
        """build_url should dispatch to sub stream."""
        url = self.strategy.build_url(
            ip="192.168.1.1", port=554, username="a", password="b",
            channel=1, stream_type="sub",
        )
        assert "02" in url  # Sub stream suffix

    def test_build_url_invalid_type_raises(self):
        """Invalid stream type should raise ValueError."""
        with pytest.raises(ValueError, match="Invalid stream_type"):
            self.strategy.build_url(
                ip="192.168.1.1", port=554, username="a", password="b",
                channel=1, stream_type="invalid",
            )


class TestDahuaStrategy:
    """Tests for Dahua RTSP URL generation."""

    def setup_method(self):
        self.strategy = DahuaStrategy()

    def test_main_stream_url(self):
        url = self.strategy.build_main_stream_url(
            ip="192.168.1.200",
            port=554,
            username="admin",
            password="admin123",
            channel=1,
        )
        assert url == "rtsp://admin:admin123@192.168.1.200:554/cam/realmonitor?channel=1&subtype=0"

    def test_sub_stream_url(self):
        url = self.strategy.build_sub_stream_url(
            ip="192.168.1.200",
            port=554,
            username="admin",
            password="admin123",
            channel=1,
        )
        assert url == "rtsp://admin:admin123@192.168.1.200:554/cam/realmonitor?channel=1&subtype=1"

    def test_multi_channel(self):
        """Different channel numbers should produce different URLs."""
        url3 = self.strategy.build_main_stream_url(
            ip="10.0.0.5", port=554, username="u", password="p", channel=3,
        )
        url8 = self.strategy.build_main_stream_url(
            ip="10.0.0.5", port=554, username="u", password="p", channel=8,
        )
        assert "channel=3" in url3
        assert "channel=8" in url8

    def test_special_chars_in_credentials(self):
        """Credentials with special chars should be URL-encoded."""
        url = self.strategy.build_main_stream_url(
            ip="192.168.1.1",
            port=554,
            username="user@domain",
            password="p@$$w0rd",
            channel=1,
        )
        assert "user%40domain" in url
        assert "p%40%24%24w0rd" in url


class TestGenericStrategy:
    """Tests for generic fallback RTSP URL generation."""

    def setup_method(self):
        self.strategy = GenericStrategy()

    def test_main_stream_url(self):
        url = self.strategy.build_main_stream_url(
            ip="192.168.1.50",
            port=554,
            username="admin",
            password="pass",
            channel=1,
        )
        assert url == "rtsp://admin:pass@192.168.1.50:554/stream1"

    def test_sub_stream_url(self):
        url = self.strategy.build_sub_stream_url(
            ip="192.168.1.50",
            port=554,
            username="admin",
            password="pass",
            channel=1,
        )
        assert url == "rtsp://admin:pass@192.168.1.50:554/stream1/sub"


class TestVendorRegistry:
    """Tests for the vendor strategy registry."""

    def test_all_vendors_registered(self):
        """All expected vendors should be in the registry."""
        assert "hikvision" in VENDOR_STRATEGIES
        assert "dahua" in VENDOR_STRATEGIES
        assert "generic" in VENDOR_STRATEGIES

    def test_get_hikvision_strategy(self):
        strategy = get_vendor_strategy("hikvision")
        assert isinstance(strategy, HikvisionStrategy)

    def test_get_dahua_strategy(self):
        strategy = get_vendor_strategy("dahua")
        assert isinstance(strategy, DahuaStrategy)

    def test_get_generic_strategy(self):
        strategy = get_vendor_strategy("generic")
        assert isinstance(strategy, GenericStrategy)

    def test_unknown_vendor_raises(self):
        """Unknown vendors should raise KeyError."""
        with pytest.raises(KeyError, match="No strategy"):
            get_vendor_strategy("unknown_vendor")

    def test_vendor_strategies_are_singletons(self):
        """Registry should return the same instance each time."""
        s1 = get_vendor_strategy("hikvision")
        s2 = get_vendor_strategy("hikvision")
        assert s1 is s2
