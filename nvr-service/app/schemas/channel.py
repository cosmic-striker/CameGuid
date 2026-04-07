"""
Pydantic schemas for NVR channel request/response serialization.

Stream URLs are NEVER exposed in responses — clients must use
signed stream tokens to access streams.
"""

from uuid import UUID

from pydantic import BaseModel, Field


# ---------------------------------------------------------------------------
# Request Schemas
# ---------------------------------------------------------------------------


class ChannelUpdate(BaseModel):
    """
    Schema for updating channel metadata.

    The main/sub stream URLs can be overridden manually for non-standard setups.
    """

    name: str | None = Field(default=None, max_length=128)
    enabled: bool | None = None
    stream_main_url: str | None = Field(
        default=None,
        max_length=512,
        description="Override generated main stream RTSP URL",
    )
    stream_sub_url: str | None = Field(
        default=None,
        max_length=512,
        description="Override generated sub stream RTSP URL",
    )


# ---------------------------------------------------------------------------
# Response Schemas
# ---------------------------------------------------------------------------


class ChannelResponse(BaseModel):
    """
    Full channel response.

    Note: stream_main_url and stream_sub_url are intentionally EXCLUDED
    from responses. Clients must use the /stream/{channel_id} endpoint
    to get a signed access token.
    """

    id: UUID
    nvr_id: UUID
    channel_number: int
    name: str | None
    enabled: bool

    model_config = {"from_attributes": True}


class ChannelListResponse(BaseModel):
    """List of channels for an NVR."""

    nvr_id: UUID
    channels: list[ChannelResponse]
    total: int


class StreamTokenResponse(BaseModel):
    """
    Signed stream access token response.

    The frontend uses this token to authenticate against the relay
    service, which internally resolves the RTSP URL.
    """

    channel_id: UUID
    stream_type: str
    token: str
    expires_in: int = Field(description="Token TTL in seconds")
