"""
Channel API routes.

Provides endpoints for listing and updating NVR channels.
Channel metadata (name, enabled state) can be edited, and stream
URLs can be manually overridden for non-standard setups.
"""

from uuid import UUID

from fastapi import APIRouter, Depends, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.database import get_db
from app.schemas.channel import (
    ChannelListResponse,
    ChannelResponse,
    ChannelUpdate,
)
from app.services import nvr_service

router = APIRouter(prefix="/nvr", tags=["Channels"])


@router.get(
    "/{nvr_id}/channels",
    response_model=ChannelListResponse,
    summary="List Channels",
    description="Get all channels for a specific NVR.",
)
async def list_channels(
    nvr_id: UUID,
    db: AsyncSession = Depends(get_db),
) -> ChannelListResponse:
    channels = await nvr_service.get_channels(db, nvr_id)
    return ChannelListResponse(
        nvr_id=nvr_id,
        channels=[ChannelResponse.model_validate(ch) for ch in channels],
        total=len(channels),
    )


@router.put(
    "/{nvr_id}/channels/{channel_id}",
    response_model=ChannelResponse,
    summary="Update Channel",
    description=(
        "Update a channel's metadata. Use this to rename channels, "
        "enable/disable them, or override stream URLs."
    ),
)
async def update_channel(
    nvr_id: UUID,
    channel_id: UUID,
    data: ChannelUpdate,
    db: AsyncSession = Depends(get_db),
) -> ChannelResponse:
    channel = await nvr_service.update_channel(db, nvr_id, channel_id, data)
    return ChannelResponse.model_validate(channel)
