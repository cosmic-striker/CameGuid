"""
Generic RTSP URL strategy (fallback).

Used for NVRs whose vendor is unknown or not explicitly supported.
Uses a conventional URL pattern that works with many ONVIF-compliant devices:
  Main:  rtsp://user:pass@ip:port/stream{channel}
  Sub:   rtsp://user:pass@ip:port/stream{channel}/sub

This is a best-effort fallback — users can always override individual
channel URLs via the channel update API.
"""

from urllib.parse import quote

from app.vendors.base import VendorStrategy


class GenericStrategy(VendorStrategy):
    """Fallback RTSP URL builder for unrecognised NVR vendors."""

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
            f"/stream{channel}"
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
            f"/stream{channel}/sub"
        )
