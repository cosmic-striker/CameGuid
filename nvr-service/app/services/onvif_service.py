"""
ONVIF integration service.

Provides an optional layer for discovering device profiles and stream
URIs via ONVIF. This is wrapped in a separate service so it can be
cleanly disabled if the ONVIF library is unavailable or unreliable.

Controlled by the NVR_ONVIF_ENABLED setting.
"""

import logging
from dataclasses import dataclass

from app.core.config import get_settings

logger = logging.getLogger(__name__)

# Try to import the ONVIF library — if unavailable, the service
# degrades gracefully to stub responses.
_ONVIF_AVAILABLE = False
try:
    from onvif import ONVIFCamera  # type: ignore[import-untyped]

    _ONVIF_AVAILABLE = True
except ImportError:
    logger.info(
        "python-onvif-zeep is not installed — ONVIF features will be disabled."
    )


# ---------------------------------------------------------------------------
# Data Classes
# ---------------------------------------------------------------------------


@dataclass
class ONVIFProfile:
    """Represents a discovered ONVIF media profile."""

    token: str
    name: str
    encoding: str | None = None
    resolution_width: int | None = None
    resolution_height: int | None = None


@dataclass
class ONVIFStreamURI:
    """A stream URI retrieved via ONVIF."""

    uri: str
    profile_token: str
    transport: str = "RTSP"


# ---------------------------------------------------------------------------
# Service Functions
# ---------------------------------------------------------------------------


def is_onvif_available() -> bool:
    """Check if ONVIF support is both installed and enabled."""
    settings = get_settings()
    return _ONVIF_AVAILABLE and settings.ONVIF_ENABLED


async def get_profiles(
    ip: str,
    port: int,
    username: str,
    password: str,
) -> list[ONVIFProfile]:
    """
    Discover ONVIF media profiles from a device.

    Args:
        ip: Device IP address.
        port: ONVIF HTTP port (usually 80).
        username: ONVIF login username.
        password: ONVIF login password.

    Returns:
        List of discovered profiles. Empty list if ONVIF is disabled
        or an error occurs.
    """
    if not is_onvif_available():
        logger.debug("ONVIF is disabled or not installed.")
        return []

    try:
        camera = ONVIFCamera(ip, port, username, password)
        await _safe_update_xaddrs(camera)

        media_service = camera.create_media_service()
        profiles = media_service.GetProfiles()

        result = []
        for profile in profiles:
            encoding = None
            width = None
            height = None

            if hasattr(profile, "VideoEncoderConfiguration") and profile.VideoEncoderConfiguration:
                vec = profile.VideoEncoderConfiguration
                encoding = str(getattr(vec, "Encoding", None))
                if hasattr(vec, "Resolution") and vec.Resolution:
                    width = getattr(vec.Resolution, "Width", None)
                    height = getattr(vec.Resolution, "Height", None)

            result.append(
                ONVIFProfile(
                    token=str(profile.token),
                    name=str(profile.Name),
                    encoding=encoding,
                    resolution_width=width,
                    resolution_height=height,
                )
            )

        logger.info("Discovered %d ONVIF profiles from %s", len(result), ip)
        return result

    except Exception as exc:
        logger.warning("ONVIF profile discovery failed for %s: %s", ip, exc)
        return []


async def get_stream_uri(
    ip: str,
    port: int,
    username: str,
    password: str,
    profile_token: str,
) -> ONVIFStreamURI | None:
    """
    Retrieve the RTSP stream URI for a specific ONVIF profile.

    Args:
        ip: Device IP address.
        port: ONVIF HTTP port.
        username: ONVIF login username.
        password: ONVIF login password.
        profile_token: ONVIF profile token to query.

    Returns:
        ONVIFStreamURI if successful, None if ONVIF is disabled or fails.
    """
    if not is_onvif_available():
        return None

    try:
        camera = ONVIFCamera(ip, port, username, password)
        await _safe_update_xaddrs(camera)

        media_service = camera.create_media_service()

        stream_setup = {
            "Stream": "RTP-Unicast",
            "Transport": {"Protocol": "RTSP"},
        }
        uri_response = media_service.GetStreamUri(
            {"StreamSetup": stream_setup, "ProfileToken": profile_token}
        )

        uri = str(uri_response.Uri)
        logger.info(
            "Retrieved ONVIF stream URI for profile '%s' from %s",
            profile_token,
            ip,
        )

        return ONVIFStreamURI(
            uri=uri,
            profile_token=profile_token,
            transport="RTSP",
        )

    except Exception as exc:
        logger.warning(
            "ONVIF stream URI retrieval failed for %s (profile=%s): %s",
            ip,
            profile_token,
            exc,
        )
        return None


async def _safe_update_xaddrs(camera: "ONVIFCamera") -> None:
    """
    Safely update the camera's service addresses.

    Some cameras don't support this — we catch and ignore failures.
    """
    try:
        camera.update_xaddrs()
    except Exception:
        pass  # Not all cameras support xaddr update
