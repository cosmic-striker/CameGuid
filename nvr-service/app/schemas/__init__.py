"""
Schemas package — re-exports all schemas for convenient imports.
"""

from app.schemas.common import ErrorResponse, SuccessResponse, HealthResponse
from app.schemas.nvr import (
    NVRCreate,
    NVRUpdate,
    NVRResponse,
    NVRDetailResponse,
    NVRListResponse,
    ValidationResult,
    GenerateChannelsResponse,
    VendorType,
)
from app.schemas.channel import (
    ChannelUpdate,
    ChannelResponse,
    ChannelListResponse,
    StreamTokenResponse,
)

__all__ = [
    "ErrorResponse",
    "SuccessResponse",
    "HealthResponse",
    "NVRCreate",
    "NVRUpdate",
    "NVRResponse",
    "NVRDetailResponse",
    "NVRListResponse",
    "ValidationResult",
    "GenerateChannelsResponse",
    "VendorType",
    "ChannelUpdate",
    "ChannelResponse",
    "ChannelListResponse",
    "StreamTokenResponse",
]
