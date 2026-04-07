"""
RTSP URL generation service.

Provides a unified interface for generating vendor-specific RTSP URLs
from NVR configuration. This service decrypts the NVR password on the
server side — raw RTSP URLs are never exposed to API clients.
"""

from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.core.exceptions import (
    ChannelNotFoundError,
    NVRNotFoundError,
    UnsupportedVendorError,
)
from app.core.security import decrypt_password
from app.models.channel import NVRChannel
from app.models.nvr import NVRDevice
from app.vendors import get_vendor_strategy


async def get_rtsp_url(
    db: AsyncSession,
    nvr_id: UUID,
    channel_number: int,
    stream_type: str = "main",
) -> str:
    """
    Generate an RTSP URL for a specific NVR channel.

    This is the unified interface mentioned in the requirements. It:
    1. Loads the NVR from the database.
    2. Decrypts the stored password.
    3. Selects the correct vendor strategy.
    4. Builds and returns the appropriate RTSP URL.

    Args:
        db: Database session.
        nvr_id: UUID of the NVR device.
        channel_number: 1-based channel number.
        stream_type: "main" or "sub".

    Returns:
        Fully-qualified RTSP URL string.

    Raises:
        NVRNotFoundError: If the NVR doesn't exist.
        UnsupportedVendorError: If the vendor strategy isn't registered.
    """
    # Load NVR
    result = await db.execute(
        select(NVRDevice).where(NVRDevice.id == nvr_id)
    )
    nvr = result.scalar_one_or_none()
    if nvr is None:
        raise NVRNotFoundError(str(nvr_id))

    return _build_rtsp_url(nvr, channel_number, stream_type)


async def get_rtsp_url_by_channel_id(
    db: AsyncSession,
    channel_id: UUID,
    stream_type: str = "main",
) -> str:
    """
    Generate an RTSP URL for a channel, looking up the NVR automatically.

    If the channel has a manually-overridden URL, that URL is used directly.
    Otherwise, the vendor strategy generates it dynamically.

    Args:
        db: Database session.
        channel_id: UUID of the channel.
        stream_type: "main" or "sub".

    Returns:
        Fully-qualified RTSP URL string.
    """
    result = await db.execute(
        select(NVRChannel)
        .options(selectinload(NVRChannel.nvr))
        .where(NVRChannel.id == channel_id)
    )
    channel = result.scalar_one_or_none()
    if channel is None:
        raise ChannelNotFoundError(str(channel_id))

    # Check for manual override first
    if stream_type == "main" and channel.stream_main_url:
        return channel.stream_main_url
    if stream_type == "sub" and channel.stream_sub_url:
        return channel.stream_sub_url

    # Fall back to vendor strategy
    nvr = channel.nvr
    if nvr is None:
        raise NVRNotFoundError()

    return _build_rtsp_url(nvr, channel.channel_number, stream_type)


def _build_rtsp_url(
    nvr: NVRDevice,
    channel_number: int,
    stream_type: str,
) -> str:
    """
    Internal helper: build RTSP URL from NVR model + vendor strategy.
    """
    # Decrypt the password
    password = decrypt_password(nvr.password_encrypted)

    # Get the vendor strategy
    try:
        strategy = get_vendor_strategy(nvr.vendor)
    except KeyError:
        raise UnsupportedVendorError(nvr.vendor)

    return strategy.build_url(
        ip=nvr.ip_address,
        port=nvr.rtsp_port,
        username=nvr.username,
        password=password,
        channel=channel_number,
        stream_type=stream_type,
    )
