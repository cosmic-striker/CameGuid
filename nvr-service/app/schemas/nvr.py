"""
Pydantic schemas for NVR device request/response serialization.

Passwords are accepted in plaintext for creation/update but are
NEVER returned in any response schema.
"""

from datetime import datetime
from enum import Enum
from uuid import UUID

from pydantic import BaseModel, Field, field_validator


class VendorType(str, Enum):
    """Supported NVR vendors."""
    hikvision = "hikvision"
    dahua = "dahua"
    generic = "generic"


# ---------------------------------------------------------------------------
# Request Schemas
# ---------------------------------------------------------------------------


class NVRCreate(BaseModel):
    """Schema for creating a new NVR device."""

    name: str = Field(..., min_length=1, max_length=128, description="Human-readable NVR name")
    ip_address: str = Field(..., description="NVR IP address (IPv4 or IPv6)")
    rtsp_port: int = Field(default=554, ge=1, le=65535, description="RTSP port")
    onvif_port: int = Field(default=80, ge=1, le=65535, description="ONVIF HTTP port")
    username: str = Field(..., min_length=1, max_length=128, description="Login username")
    password: str = Field(..., min_length=1, description="Login password (will be encrypted)")
    vendor: VendorType = Field(default=VendorType.generic, description="NVR vendor/manufacturer")
    channel_count: int = Field(default=4, ge=1, le=128, description="Number of video channels")

    @field_validator("ip_address")
    @classmethod
    def validate_ip(cls, v: str) -> str:
        """Basic IP address format validation."""
        v = v.strip()
        if not v:
            raise ValueError("IP address cannot be empty.")
        # Accept IPv4, IPv6, or hostnames for flexibility
        return v


class NVRUpdate(BaseModel):
    """
    Schema for updating an existing NVR device.
    
    All fields are optional — only provided fields are updated.
    """

    name: str | None = Field(default=None, min_length=1, max_length=128)
    ip_address: str | None = Field(default=None)
    rtsp_port: int | None = Field(default=None, ge=1, le=65535)
    onvif_port: int | None = Field(default=None, ge=1, le=65535)
    username: str | None = Field(default=None, min_length=1, max_length=128)
    password: str | None = Field(default=None, min_length=1)
    vendor: VendorType | None = None
    channel_count: int | None = Field(default=None, ge=1, le=128)


# ---------------------------------------------------------------------------
# Response Schemas
# ---------------------------------------------------------------------------


class NVRResponse(BaseModel):
    """
    NVR device response — password is NEVER included.
    """

    id: UUID
    name: str
    ip_address: str
    rtsp_port: int
    onvif_port: int
    username: str
    vendor: VendorType
    channel_count: int
    created_at: datetime
    updated_at: datetime | None = None

    model_config = {"from_attributes": True}


class NVRDetailResponse(NVRResponse):
    """NVR response with nested channel list."""

    channels: list["ChannelBriefResponse"] = []


class ChannelBriefResponse(BaseModel):
    """Minimal channel info for embedding in NVR detail responses."""

    id: UUID
    channel_number: int
    name: str | None
    enabled: bool

    model_config = {"from_attributes": True}


class NVRListResponse(BaseModel):
    """Paginated NVR list response."""

    data: list[NVRResponse]
    total: int


class ValidationResult(BaseModel):
    """Result of an RTSP connection validation test."""

    success: bool
    latency_ms: float | None = None
    error: str | None = None
    codec_info: dict | None = None


class GenerateChannelsResponse(BaseModel):
    """Response from channel generation."""

    message: str
    channels_created: int
    channels: list["ChannelBriefResponse"] = []


class DiscoverChannelsResponse(BaseModel):
    """Response from real-time NVR channel auto-discovery."""

    message: str
    channels_created: int
    channels: list["ChannelBriefResponse"] = []


# Resolve forward references
NVRDetailResponse.model_rebuild()
GenerateChannelsResponse.model_rebuild()
DiscoverChannelsResponse.model_rebuild()
