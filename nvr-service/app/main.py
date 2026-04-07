"""
FastAPI application entrypoint for the NVR integration service.

Configures the application, registers routers and exception handlers,
and manages database lifecycle (startup/shutdown).
"""

import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.router import api_router
from app.core.config import get_settings
from app.core.database import close_db, init_db
from app.core.exceptions import NVRServiceError, nvr_exception_handler
from app.schemas.common import HealthResponse

# Configure logging
logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s | %(levelname)-7s | %(name)s | %(message)s",
    datefmt="%Y-%m-%d %H:%M:%S",
)
logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Lifespan (startup / shutdown)
# ---------------------------------------------------------------------------


@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifespan: initialise database on startup, close on shutdown."""
    settings = get_settings()
    logger.info(
        "Starting NVR Service on %s:%d (log_level=%s)",
        settings.HOST,
        settings.PORT,
        settings.LOG_LEVEL,
    )

    # Create tables if they don't exist
    # In production, use Alembic migrations instead
    await init_db()
    logger.info("Database initialised.")

    yield

    # Shutdown
    await close_db()
    logger.info("NVR Service shut down.")


# ---------------------------------------------------------------------------
# Application Factory
# ---------------------------------------------------------------------------


def create_app() -> FastAPI:
    """Create and configure the FastAPI application."""
    app = FastAPI(
        title="CamGuid NVR Integration Service",
        description=(
            "Production-ready NVR management module for the CamGuid CCTV monitoring system. "
            "Supports Hikvision, Dahua, and generic NVR vendors with encrypted credentials, "
            "RTSP URL generation, connection validation, and signed stream access."
        ),
        version="1.0.0",
        lifespan=lifespan,
        docs_url="/docs",
        redoc_url="/redoc",
    )

    # --- CORS ---
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"],
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )

    # --- Exception Handlers ---
    app.add_exception_handler(NVRServiceError, nvr_exception_handler)

    # --- Routers ---
    app.include_router(api_router)

    # --- Health Check ---
    @app.get(
        "/health",
        response_model=HealthResponse,
        tags=["System"],
        summary="Health Check",
    )
    async def health_check() -> HealthResponse:
        return HealthResponse(status="ok", service="nvr-service")

    return app


def get_app() -> FastAPI:
    """
    Lazy app singleton for uvicorn.

    Defers config validation to first call, which happens at server startup
    (not at import time), making tests safe to import create_app directly.
    """
    global _app_instance
    if _app_instance is None:
        _app_instance = create_app()
    return _app_instance


_app_instance: FastAPI | None = None

# Used by uvicorn: `uvicorn app.main:app`
# This is a callable that uvicorn will invoke to get the app
app = get_app


# ---------------------------------------------------------------------------
# Direct execution
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    import uvicorn

    settings = get_settings()
    uvicorn.run(
        "app.main:app",
        host=settings.HOST,
        port=settings.PORT,
        reload=True,
        log_level=settings.LOG_LEVEL,
        factory=True,
    )
