"""
Stream access API routes.

Provides a secure endpoint for obtaining signed stream access tokens.
The frontend requests a token for a specific channel, then uses that
token to authenticate against the relay service. Raw RTSP URLs are
NEVER returned to the client.
"""

from uuid import UUID

from fastapi import APIRouter, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.config import get_settings
from app.core.database import get_db
from app.core.security import create_stream_token, verify_stream_token
from app.core.exceptions import ChannelNotFoundError, StreamTokenError
from app.schemas.channel import StreamTokenResponse
from app.services import nvr_service, rtsp_service

router = APIRouter(prefix="/stream", tags=["Stream"])


@router.get(
    "/{channel_id}",
    response_model=StreamTokenResponse,
    summary="Get Stream Access Token",
    description=(
        "Request a signed, time-limited access token for a channel's stream. "
        "The token can be presented to the relay service to start streaming. "
        "Raw RTSP URLs are never exposed to the client."
    ),
)
async def get_stream_token(
    channel_id: UUID,
    type: str = Query(
        default="main",
        description="Stream type: 'main' (full resolution) or 'sub' (low resolution).",
        pattern="^(main|sub)$",
    ),
    db: AsyncSession = Depends(get_db),
) -> StreamTokenResponse:
    settings = get_settings()

    # Verify the channel exists and has a valid NVR
    channel = await nvr_service.get_channel(db, channel_id)

    # Generate signed token
    token = create_stream_token(
        channel_id=str(channel_id),
        stream_type=type,
        ttl=settings.STREAM_TOKEN_TTL,
    )

    return StreamTokenResponse(
        channel_id=channel_id,
        stream_type=type,
        token=token,
        expires_in=settings.STREAM_TOKEN_TTL,
    )


@router.post(
    "/resolve",
    summary="Resolve Stream Token (Internal)",
    description=(
        "Internal endpoint used by the relay service to resolve a stream "
        "token into an RTSP URL. This should NOT be exposed to the frontend."
    ),
    include_in_schema=False,  # Hidden from OpenAPI docs
)
async def resolve_stream_token(
    token: str = Query(..., description="Signed stream access token"),
    db: AsyncSession = Depends(get_db),
) -> dict:
    """
    Resolve a signed stream token to the actual RTSP URL.

    This is an internal endpoint meant to be called by the relay service
    when it needs to know which RTSP URL to connect to for a given token.
    """
    try:
        payload = verify_stream_token(token)
    except ValueError as exc:
        raise StreamTokenError(str(exc))

    channel_id = payload["cid"]
    stream_type = payload.get("type", "main")

    url = await rtsp_service.get_rtsp_url_by_channel_id(
        db, UUID(channel_id), stream_type
    )

    return {
        "rtsp_url": url,
        "channel_id": channel_id,
        "stream_type": stream_type,
    }
