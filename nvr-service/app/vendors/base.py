"""
Abstract base class for vendor-specific RTSP URL generation strategies.

Each NVR vendor has its own URL format for accessing video channels.
Subclass VendorStrategy and implement the two abstract methods to add
support for a new vendor.
"""

from abc import ABC, abstractmethod


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
