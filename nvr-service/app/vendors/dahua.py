"""
Dahua RTSP URL strategy.

Dahua NVRs use the following URL pattern:
  Main:  rtsp://user:pass@ip:port/cam/realmonitor?channel={channel}&subtype=0
  Sub:   rtsp://user:pass@ip:port/cam/realmonitor?channel={channel}&subtype=1

Where {channel} is the 1-based channel number and subtype 0/1 denote
main/sub stream respectively.

Reference: Dahua SDK / media streaming documentation.
"""

import logging
import re
from urllib.parse import quote, urlencode

from app.vendors.base import DiscoveredChannel, VendorStrategy
from app.vendors.http_client import get_with_auth

logger = logging.getLogger(__name__)

# Dahua CGI endpoint that reports per-channel titles. Widely supported across
# Dahua NVR/DVR firmware generations.
DAHUA_CHANNEL_TITLE_URI = "/cgi-bin/configManager.cgi"

_dahua_channel_title_re = re.compile(r"ChannelTitle\.(\d+)=(.*)")


def parse_dahua_channels(text: str) -> list[DiscoveredChannel]:
    """
    Parse a Dahua `configManager.cgi?action=getConfig&name=ChannelTitle` response.

    The CGI returns one `ChannelTitle.{N}={name}` line per channel. Channels
    with an empty title typically have no camera connected, so only channels
    with a non-empty title are kept. If that leaves nothing (e.g. the response
    was empty/unknown), the full parsed set is returned so the caller can fall
    back to channel_count generation.

    Args:
        text: Raw CGI response body.

    Returns:
        List of discovered channels.
    """
    channels: list[DiscoveredChannel] = []
    for line in text.splitlines():
        line = line.strip()
        match = _dahua_channel_title_re.match(line)
        if not match:
            continue
        try:
            channel_number = int(match.group(1))
        except ValueError:
            continue
        name = match.group(2).strip()
        channels.append(
            DiscoveredChannel(
                channel_number=channel_number,
                name=name or None,
                enabled=True,
            )
        )

    channels.sort(key=lambda c: c.channel_number)

    named = [c for c in channels if c.name]
    return named if named else channels


class DahuaStrategy(VendorStrategy):
    """RTSP URL builder for Dahua NVRs."""

    async def discover_channels(
        self,
        ip: str,
        http_port: int,
        rtsp_port: int,
        username: str,
        password: str,
    ) -> list[DiscoveredChannel] | None:
        """
        Query the Dahua CGI `ChannelTitle` config to enumerate real channels.

        Returns None only on transport-level failure (unreachable NVR), which
        lets the caller decide how to handle it. An empty parseable result
        falls through to the channel_count fallback.

        Args:
            ip: NVR IP address.
            http_port: NVR management HTTP port (typically 80).
            rtsp_port: RTSP port — unused for discovery, kept for the contract.
            username: NVR login username.
            password: NVR login password (plaintext).

        Returns:
            Discovered channels, or None if the NVR could not be reached.
        """
        query = urlencode(
            {"action": "getConfig", "name": "ChannelTitle"}
        )
        url = f"http://{ip}:{http_port}{DAHUA_CHANNEL_TITLE_URI}?{query}"
        logger.info("Discovering Dahua channels from %s", url)

        try:
            response = await get_with_auth(url, username, password)
        except Exception as exc:
            logger.warning("Dahua discovery request failed for %s: %s", ip, exc)
            return None

        if response.status_code != 200:
            logger.warning(
                "Dahua discovery returned HTTP %s for %s",
                response.status_code,
                ip,
            )
            return None

        return parse_dahua_channels(response.text)

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
