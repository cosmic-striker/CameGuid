"""
Dahua RTSP URL strategy.

Dahua NVRs use the following URL pattern:
  Main:  rtsp://user:pass@ip:port/cam/realmonitor?channel={channel}&subtype=0
  Sub:   rtsp://user:pass@ip:port/cam/realmonitor?channel={channel}&subtype=1

Where {channel} is the 1-based channel number and subtype 0/1 denote
main/sub stream respectively.

Reference: Dahua SDK / media streaming documentation.
"""

from urllib.parse import quote

from app.vendors.base import VendorStrategy


class DahuaStrategy(VendorStrategy):
    """RTSP URL builder for Dahua NVRs."""

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
            f"/cam/realmonitor?channel={channel}&subtype=0"
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
            f"/cam/realmonitor?channel={channel}&subtype=1"
        )
