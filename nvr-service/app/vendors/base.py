"""
Abstract base class for vendor-specific RTSP URL generation strategies.

Each NVR vendor has its own URL format for accessing video channels.
Subclass VendorStrategy and implement the two abstract methods to add
support for a new vendor.
"""

from abc import ABC, abstractmethod
from dataclasses import dataclass, field


@dataclass
class DiscoveredChannel:
    """
    A camera channel enumerated in real time from an NVR.

    This represents a channel the NVR reports as actually connected,
    as opposed to channels blindly generated 1..channel_count.
    """

    channel_number: int
    name: str | None = None
    enabled: bool = True
    online: bool | None = None


class VendorStrategy(ABC):
    """
    Abstract strategy for building vendor-specific RTSP URLs.

    Implementations must handle both main stream (full resolution) and
    sub stream (lower resolution / mobile) URL formats.
    """

    @abstractmethod
    def build_main_stream_url(
        self,
        ip: str,
        port: int,
        username: str,
        password: str,
        channel: int,
    ) -> str:
        """
        Build the main (primary / high-res) stream RTSP URL.

        Args:
            ip: NVR IP address.
            port: RTSP port (usually 554).
            username: NVR login username.
            password: NVR login password (plaintext).
            channel: 1-based channel number.

        Returns:
            Fully-qualified RTSP URL string.
        """
        ...

    @abstractmethod
    def build_sub_stream_url(
        self,
        ip: str,
        port: int,
        username: str,
        password: str,
        channel: int,
    ) -> str:
        """
        Build the sub (secondary / low-res) stream RTSP URL.

        Args:
            ip: NVR IP address.
            port: RTSP port (usually 554).
            username: NVR login username.
            password: NVR login password (plaintext).
            channel: 1-based channel number.

        Returns:
            Fully-qualified RTSP URL string.
        """
        ...

    def build_url(
        self,
        ip: str,
        port: int,
        username: str,
        password: str,
        channel: int,
        stream_type: str = "main",
    ) -> str:
        """
        Convenience method — selects main or sub stream builder.

        Args:
            stream_type: "main" or "sub".

        Raises:
            ValueError: If stream_type is not "main" or "sub".
        """
        if stream_type == "main":
            return self.build_main_stream_url(ip, port, username, password, channel)
        elif stream_type == "sub":
            return self.build_sub_stream_url(ip, port, username, password, channel)
        else:
            raise ValueError(
                f"Invalid stream_type '{stream_type}'. Must be 'main' or 'sub'."
            )

    async def discover_channels(
        self,
        ip: str,
        http_port: int,
        rtsp_port: int,
        username: str,
        password: str,
    ) -> list[DiscoveredChannel] | None:
        """
        Optionally enumerate the camera channels actually connected to an NVR.

        Vendor strategies that expose an HTTP API (Hikvision ISAPI, Dahua CGI)
        override this to query the device. Strategies without a discovery
        mechanism return None, in which case the discovery service falls back
        to generating channels from the NVR's configured channel_count.

        Args:
            ip: NVR IP address.
            http_port: NVR management HTTP port (typically 80).
            rtsp_port: NVR RTSP port (typically 554).
            username: NVR login username.
            password: NVR login password (plaintext).

        Returns:
            List of discovered channels, or None to signal "not supported" so
            the caller can fall back to channel_count generation.
        """
        return None
