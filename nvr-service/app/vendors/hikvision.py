"""
Hikvision RTSP URL strategy.

Hikvision NVRs use the following URL pattern:
  Main:  rtsp://user:pass@ip:port/Streaming/Channels/{channel}01
  Sub:   rtsp://user:pass@ip:port/Streaming/Channels/{channel}02

Where {channel} is the 1-based channel number and 01/02 denote
main/sub stream respectively.

Reference: Hikvision ISAPI documentation.
"""

from urllib.parse import quote

from app.vendors.base import VendorStrategy


class HikvisionStrategy(VendorStrategy):
    """RTSP URL builder for Hikvision NVRs."""

    def build_main_stream_url(
        self,
        ip: str,
        port: int,
        username: str,
        password: str,
        channel: int,
    ) -> str:
        user_encoded = quote(username, safe="")
        pass_encoded = quote(password, safe="")
        return (
            f"rtsp://{user_encoded}:{pass_encoded}@{ip}:{port}"
            f"/Streaming/Channels/{channel}01"
        )

    def build_sub_stream_url(
        self,
        ip: str,
        port: int,
        username: str,
        password: str,
        channel: int,
    ) -> str:
        user_encoded = quote(username, safe="")
        pass_encoded = quote(password, safe="")
        return (
            f"rtsp://{user_encoded}:{pass_encoded}@{ip}:{port}"
            f"/Streaming/Channels/{channel}02"
        )
