"""
NVR channel auto-discovery service.

Discovers the camera channels actually connected to an NVR by querying the
vendor management API (Hikvision ISAPI / Dahua CGI), then persists them as
NVRChannel rows. This is the "NVR-first" flow: the operator adds the NVR once
and gets every connected camera back — no per-camera credential entry.

If a vendor has no discovery endpoint (generic) or the query comes up empty,
the service falls back to generating channels from the NVR's configured
channel_count so the platform never hard-blocks on vendor API quirks.
"""

import logging
from uuid import UUID

from sqlalchemy import delete
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.exceptions import UnsupportedVendorError
from app.core.security import decrypt_password
from app.models.channel import NVRChannel
from app.services import nvr_service
from app.vendors import get_vendor_strategy

logger = logging.getLogger(__name__)


async def discover_nvr_channels(
    db: AsyncSession,
    nvr_id: UUID,
) -> list[NVRChannel]:
    """
    Auto-discover the camera channels connected to an NVR and persist them.

    Existing channels are replaced by the discovered set. The stored
    channel_count is synced to the real discovered count so the UI reflects
    actual connected cameras rather than a manually-entered guess.

    Args:
        db: Database session.
        nvr_id: UUID of the NVR device.

    Returns:
        The list of persisted NVRChannel rows.

    Raises:
        NVRNotFoundError: If the NVR doesn't exist (via nvr_service.get_nvr).
        UnsupportedVendorError: If the vendor isn't registered.
        ChannelNotFoundError: Not raised here; kept for parity with services.
    """
    nvr = await nvr_service.get_nvr(db, nvr_id)

    password = decrypt_password(nvr.password_encrypted)

    try:
        strategy = get_vendor_strategy(nvr.vendor)
    except KeyError:
        raise UnsupportedVendorError(nvr.vendor)

    # Ask the NVR for its real channel list (vendors without an API return
    # None, which triggers the fallback path below).
    discovered: list | None = None
    if nvr.vendor != "generic":
        discovered = await strategy.discover_channels(
            ip=nvr.ip_address,
            http_port=nvr.onvif_port,
            rtsp_port=nvr.rtsp_port,
            username=nvr.username,
            password=password,
        )

    if not discovered:
        logger.info(
            "No channels auto-discovered for '%s' — falling back to "
            "channel_count=%d",
            nvr.name,
            nvr.channel_count,
        )
        return await nvr_service.generate_channels(db, nvr_id)

    # Replace existing channels with the discovered set
    await db.execute(delete(NVRChannel).where(NVRChannel.nvr_id == nvr_id))

    channels: list[NVRChannel] = []
    for ch in discovered:
        main_url = strategy.build_main_stream_url(
            ip=nvr.ip_address,
            port=nvr.rtsp_port,
            username=nvr.username,
            password=password,
            channel=ch.channel_number,
        )
        sub_url = strategy.build_sub_stream_url(
            ip=nvr.ip_address,
            port=nvr.rtsp_port,
            username=nvr.username,
            password=password,
            channel=ch.channel_number,
        )

        channel = NVRChannel(
            nvr_id=nvr.id,
            channel_number=ch.channel_number,
            name=ch.name or f"{nvr.name} - CH{ch.channel_number}",
            stream_main_url=main_url,
            stream_sub_url=sub_url,
            enabled=ch.enabled,
        )
        db.add(channel)
        channels.append(channel)

    await db.flush()

    # Refresh to populate generated IDs/timestamps
    for ch in channels:
        await db.refresh(ch)

    # Sync channel_count to reality so the dashboard never shows a guess
    if nvr.channel_count != len(channels):
        nvr.channel_count = len(channels)
        await db.flush()

    logger.info(
        "Discovered and persisted %d channels for NVR '%s' (id=%s)",
        len(channels),
        nvr.name,
        nvr.id,
    )
    return channels