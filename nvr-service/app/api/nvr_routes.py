"""
NVR device API routes.

Provides CRUD endpoints for NVR management, connection validation,
and channel generation. All business logic is delegated to the
service layer — these handlers only handle HTTP concerns.
"""

from uuid import UUID

from fastapi import APIRouter, Depends, status
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.database import get_db
from app.schemas.nvr import (
    GenerateChannelsResponse,
    NVRCreate,
    NVRDetailResponse,
    NVRListResponse,
    NVRResponse,
    NVRUpdate,
    ValidationResult,
)
from app.schemas.channel import ChannelResponse
from app.schemas.common import SuccessResponse
from app.services import nvr_service, rtsp_service, validation_service

router = APIRouter(prefix="/nvr", tags=["NVR"])


@router.post(
    "",
    response_model=NVRResponse,
    status_code=status.HTTP_201_CREATED,
    summary="Create NVR",
    description="Register a new NVR device. The password will be encrypted before storage.",
)
async def create_nvr(
    data: NVRCreate,
    db: AsyncSession = Depends(get_db),
) -> NVRResponse:
    nvr = await nvr_service.create_nvr(db, data)
    return NVRResponse.model_validate(nvr)


@router.get(
    "",
    response_model=NVRListResponse,
    summary="List NVRs",
    description="Retrieve all registered NVR devices.",
)
async def list_nvrs(
    db: AsyncSession = Depends(get_db),
) -> NVRListResponse:
    nvrs = await nvr_service.list_nvrs(db)
    return NVRListResponse(
        data=[NVRResponse.model_validate(n) for n in nvrs],
        total=len(nvrs),
    )


@router.get(
    "/{nvr_id}",
    response_model=NVRDetailResponse,
    summary="Get NVR",
    description="Retrieve a single NVR by ID, including its channels.",
)
async def get_nvr(
    nvr_id: UUID,
    db: AsyncSession = Depends(get_db),
) -> NVRDetailResponse:
    nvr = await nvr_service.get_nvr(db, nvr_id)
    return NVRDetailResponse.model_validate(nvr)


@router.put(
    "/{nvr_id}",
    response_model=NVRResponse,
    summary="Update NVR",
    description="Update an existing NVR. Only provided fields are modified.",
)
async def update_nvr(
    nvr_id: UUID,
    data: NVRUpdate,
    db: AsyncSession = Depends(get_db),
) -> NVRResponse:
    nvr = await nvr_service.update_nvr(db, nvr_id, data)
    return NVRResponse.model_validate(nvr)


@router.delete(
    "/{nvr_id}",
    status_code=status.HTTP_204_NO_CONTENT,
    summary="Delete NVR",
    description="Delete an NVR and all its channels.",
)
async def delete_nvr(
    nvr_id: UUID,
    db: AsyncSession = Depends(get_db),
) -> None:
    await nvr_service.delete_nvr(db, nvr_id)


@router.post(
    "/{nvr_id}/validate",
    response_model=ValidationResult,
    summary="Validate NVR Connection",
    description=(
        "Test RTSP connectivity to an NVR by probing channel 1. "
        "Returns success/failure, latency, and codec information."
    ),
)
async def validate_nvr(
    nvr_id: UUID,
    db: AsyncSession = Depends(get_db),
) -> ValidationResult:
    # Generate the RTSP URL for channel 1, main stream
    url = await rtsp_service.get_rtsp_url(db, nvr_id, channel_number=1, stream_type="main")
    result = await validation_service.validate_rtsp_connection(url)
    return result


@router.post(
    "/{nvr_id}/generate-channels",
    response_model=GenerateChannelsResponse,
    summary="Generate Channels",
    description=(
        "Auto-generate channels for an NVR based on its channel_count. "
        "Existing channels will be replaced."
    ),
)
async def generate_channels(
    nvr_id: UUID,
    db: AsyncSession = Depends(get_db),
) -> GenerateChannelsResponse:
    channels = await nvr_service.generate_channels(db, nvr_id)
    return GenerateChannelsResponse(
        message=f"Generated {len(channels)} channels.",
        channels_created=len(channels),
        channels=[
            ChannelResponse.model_validate(ch).__dict__
            for ch in channels
        ],
    )
