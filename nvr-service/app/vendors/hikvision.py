"""
Hikvision RTSP URL strategy.

Hikvision NVRs use the following URL pattern:
  Main:  rtsp://user:pass@ip:port/Streaming/Channels/{channel}01
  Sub:   rtsp://user:pass@ip:port/Streaming/Channels/{channel}02

Where {channel} is the 1-based channel number and 01/02 denote
main/sub stream respectively.

Reference: Hikvision ISAPI documentation.
"""

import logging
import xml.etree.ElementTree as ET
from urllib.parse import quote

from app.vendors.base import DiscoveredChannel, VendorStrategy
from app.vendors.http_client import get_with_auth

logger = logging.getLogger(__name__)

# Hikvision ISAPI endpoint that lists the IP cameras connected to an NVR.
ISAPI_INPUT_CHANNELS_PATH = "/ISAPI/ContentMgmt/InputProxy/channels"


def parse_hikvision_channels(xml_text: str) -> list[DiscoveredChannel]:
    """
    Parse a Hikvision `InputProxyChannelList` XML response.

    The list only contains channels the NVR actually detects, so parsing it
    gives the real connected camera count (as opposed to guessing 1..N).

    Args:
        xml_text: Raw ISAPI response body.

    Returns:
        List of discovered channels. Empty on any parse failure.
    """
    try:
        root = ET.fromstring(xml_text)
    except ET.ParseError as exc:
        logger.debug("Hikvision channel XML unparseable: %s", exc)
        return []

    channels: list[DiscoveredChannel] = []
    for element in root.iter():
        tag = element.tag.rsplit("}", 1)[-1]
        if tag != "InputProxyChannel":
            continue

        channel_number = None
        name = None
        enabled = True
        online = None
        for child in element:
            child_tag = child.tag.rsplit("}", 1)[-1]
            if child_tag == "id":
                try:
                    channel_number = int(child.text or "")
                except ValueError:
                    channel_number = None
            elif child_tag == "name":
                name = child.text.strip() if child.text else None
            elif child_tag == "enabled":
                enabled = (child.text or "").strip().lower() in (
                    "true", "1", "yes", "on"
                )
            elif child_tag == "online":
                online = (child.text or "").strip().lower() in (
                    "true", "1", "yes", "on"
                )

        if channel_number is not None:
            channels.append(
                DiscoveredChannel(
                    channel_number=channel_number,
                    name=name,
                    enabled=enabled,
                    online=online,
                )
            )

    return channels


class HikvisionStrategy(VendorStrategy):
    """RTSP URL builder for Hikvision NVRs."""

    async def discover_channels(
        self,
        ip: str,
        http_port: int,
        rtsp_port: int,
        username: str,
        password: str,
    ) -> list[DiscoveredChannel] | None:
        """
        Query the Hikvision ISAPI `InputProxy/channels` endpoint.

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
        url = f"http://{ip}:{http_port}{ISAPI_INPUT_CHANNELS_PATH}"
        logger.info("Discovering Hikvision channels from %s", url)

        try:
            response = await get_with_auth(url, username, password)
        except Exception as exc:
            logger.warning("Hikvision discovery request failed for %s: %s", ip, exc)
            return None

        if response.status_code != 200:
            logger.warning(
                "Hikvision discovery returned HTTP %s for %s",
                response.status_code,
                ip,
            )
            return None

        return parse_hikvision_channels(response.text)

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
