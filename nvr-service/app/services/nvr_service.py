"""
NVR CRUD service.

Business logic layer for managing NVR devices and their channels.
All password encryption/decryption is handled here — controllers
never touch raw passwords or ciphertext directly.
"""

import logging
from uuid import UUID

from sqlalchemy import delete, func, select
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from app.core.exceptions import (
    InvalidConfigurationError,
    NVRNotFoundError,
    UnsupportedVendorError,
)
from app.core.security import decrypt_password, encrypt_password
from app.models.channel import NVRChannel
from app.models.nvr import NVRDevice
from app.schemas.nvr import NVRCreate, NVRUpdate
from app.services import rtsp_service
from app.vendors import get_vendor_strategy

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# NVR CRUD
# ---------------------------------------------------------------------------


async def create_nvr(db: AsyncSession, data: NVRCreate) -> NVRDevice:
    """
    Create a new NVR device.

    The password is encrypted before storage. The vendor is validated
    against the registered strategy list.
    """
    # Validate vendor
    try:
        get_vendor_strategy(data.vendor.value)
    except KeyError:
        raise UnsupportedVendorError(data.vendor.value)

    # Encrypt password
    encrypted_pw = encrypt_password(data.password)

    nvr = NVRDevice(
        name=data.name,
        ip_address=data.ip_address,
        rtsp_port=data.rtsp_port,
        onvif_port=data.onvif_port,
        username=data.username,
        password_encrypted=encrypted_pw,
        vendor=data.vendor.value,
        channel_count=data.channel_count,
    )

    db.add(nvr)
    await db.flush()
    await db.refresh(nvr)

    logger.info("Created NVR '%s' (id=%s, vendor=%s)", nvr.name, nvr.id, nvr.vendor)
    return nvr


async def get_nvr(db: AsyncSession, nvr_id: UUID) -> NVRDevice:
    """
    Retrieve a single NVR by ID.

    Raises NVRNotFoundError if not found.
    """
    result = await db.execute(
        select(NVRDevice)
        .options(selectinload(NVRDevice.channels))
        .where(NVRDevice.id == nvr_id)
    )
    nvr = result.scalar_one_or_none()
    if nvr is None:
        raise NVRNotFoundError(str(nvr_id))
    return nvr


async def list_nvrs(db: AsyncSession) -> list[NVRDevice]:
    """List all NVR devices, ordered by creation date."""
    result = await db.execute(
        select(NVRDevice).order_by(NVRDevice.created_at.desc())
    )
    return list(result.scalars().all())


async def update_nvr(
    db: AsyncSession,
    nvr_id: UUID,
    data: NVRUpdate,
) -> NVRDevice:
    """
    Update an existing NVR. Only provided (non-None) fields are updated.

    If the password is updated, the new password is encrypted before storage.
    """
    nvr = await get_nvr(db, nvr_id)

    update_data = data.model_dump(exclude_unset=True)

    # Handle password separately — needs encryption
    if "password" in update_data:
        new_password = update_data.pop("password")
        if new_password is not None:
            nvr.password_encrypted = encrypt_password(new_password)

    # Validate vendor if being changed
    if "vendor" in update_data and update_data["vendor"] is not None:
        vendor_val = update_data["vendor"]
        if hasattr(vendor_val, "value"):
            vendor_val = vendor_val.value
        try:
            get_vendor_strategy(vendor_val)
        except KeyError:
            raise UnsupportedVendorError(vendor_val)
        update_data["vendor"] = vendor_val

    # Apply remaining updates
    for field, value in update_data.items():
        if value is not None and hasattr(nvr, field):
            setattr(nvr, field, value)

    await db.flush()
    await db.refresh(nvr)

    logger.info("Updated NVR '%s' (id=%s)", nvr.name, nvr.id)
    return nvr


async def delete_nvr(db: AsyncSession, nvr_id: UUID) -> None:
    """
    Delete an NVR and all its channels (via cascade).

    Raises NVRNotFoundError if not found.
    """
    nvr = await get_nvr(db, nvr_id)
    await db.delete(nvr)
    await db.flush()
    logger.info("Deleted NVR '%s' (id=%s)", nvr.name, nvr.id)


# ---------------------------------------------------------------------------
# Channel Management
# ---------------------------------------------------------------------------


async def generate_channels(db: AsyncSession, nvr_id: UUID) -> list[NVRChannel]:
    """
    Auto-generate channels for an NVR based on its channel_count.

    Existing channels are deleted and regenerated. Stream URLs are
    computed using the NVR's vendor strategy.

    Returns:
        List of newly created NVRChannel objects.
    """
    nvr = await get_nvr(db, nvr_id)

    # Delete existing channels
    await db.execute(
        delete(NVRChannel).where(NVRChannel.nvr_id == nvr_id)
    )

    # Decrypt password for URL generation
    password = decrypt_password(nvr.password_encrypted)

    # Get vendor strategy
    try:
        strategy = get_vendor_strategy(nvr.vendor)
    except KeyError:
        raise UnsupportedVendorError(nvr.vendor)

    # Generate channels
    channels = []
    for ch_num in range(1, nvr.channel_count + 1):
        main_url = strategy.build_main_stream_url(
            ip=nvr.ip_address,
            port=nvr.rtsp_port,
            username=nvr.username,
            password=password,
            channel=ch_num,
        )
        sub_url = strategy.build_sub_stream_url(
            ip=nvr.ip_address,
            port=nvr.rtsp_port,
            username=nvr.username,
            password=password,
            channel=ch_num,
        )

        channel = NVRChannel(
            nvr_id=nvr.id,
            channel_number=ch_num,
            name=f"{nvr.name} - CH{ch_num}",
            stream_main_url=main_url,
            stream_sub_url=sub_url,
            enabled=True,
        )
        db.add(channel)
        channels.append(channel)

    await db.flush()

    # Refresh to get generated IDs
    for ch in channels:
        await db.refresh(ch)

    logger.info(
        "Generated %d channels for NVR '%s' (id=%s)",
        len(channels),
        nvr.name,
        nvr.id,
    )
    return channels


async def get_channels(db: AsyncSession, nvr_id: UUID) -> list[NVRChannel]:
    """Get all channels for an NVR, ordered by channel number."""
    # Verify NVR exists
    await get_nvr(db, nvr_id)

    result = await db.execute(
        select(NVRChannel)
        .where(NVRChannel.nvr_id == nvr_id)
        .order_by(NVRChannel.channel_number)
    )
    return list(result.scalars().all())


async def get_channel(db: AsyncSession, channel_id: UUID) -> NVRChannel:
    """Get a single channel by ID."""
    from app.core.exceptions import ChannelNotFoundError

    result = await db.execute(
        select(NVRChannel)
        .options(selectinload(NVRChannel.nvr))
        .where(NVRChannel.id == channel_id)
    )
    channel = result.scalar_one_or_none()
    if channel is None:
        raise ChannelNotFoundError(str(channel_id))
    return channel


async def update_channel(
    db: AsyncSession,
    nvr_id: UUID,
    channel_id: UUID,
    data: "ChannelUpdate",
) -> NVRChannel:
    """
    Update a channel's metadata (name, enabled, URL overrides).

    The channel must belong to the specified NVR.
    """
    from app.core.exceptions import ChannelNotFoundError
    from app.schemas.channel import ChannelUpdate

    result = await db.execute(
        select(NVRChannel).where(
            NVRChannel.id == channel_id,
            NVRChannel.nvr_id == nvr_id,
        )
    )
    channel = result.scalar_one_or_none()
    if channel is None:
        raise ChannelNotFoundError(str(channel_id))

    update_data = data.model_dump(exclude_unset=True)
    for field, value in update_data.items():
        if hasattr(channel, field):
            setattr(channel, field, value)

    await db.flush()
    await db.refresh(channel)

    logger.info(
        "Updated channel '%s' (id=%s, nvr=%s)",
        channel.name,
        channel.id,
        nvr_id,
    )
    return channel
