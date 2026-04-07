"""
Main API router.

Aggregates all sub-routers under the /api/v1 prefix.
"""

from fastapi import APIRouter

from app.api.nvr_routes import router as nvr_router
from app.api.channel_routes import router as channel_router
from app.api.stream_routes import router as stream_router

api_router = APIRouter(prefix="/api/v1")

api_router.include_router(nvr_router)
api_router.include_router(channel_router)
api_router.include_router(stream_router)
