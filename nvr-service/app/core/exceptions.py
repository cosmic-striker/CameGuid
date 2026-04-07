"""
Structured exception classes for the NVR service.

Each exception maps to a specific HTTP status code and error code string,
enabling the global exception handler to produce consistent API responses.
"""

from fastapi import Request
from fastapi.responses import JSONResponse


# ---------------------------------------------------------------------------
# Base Exception
# ---------------------------------------------------------------------------


class NVRServiceError(Exception):
    """Base exception for all NVR service errors."""

    status_code: int = 500
    error_code: str = "INTERNAL_ERROR"

    def __init__(self, message: str = "An internal error occurred."):
        self.message = message
        super().__init__(self.message)


# ---------------------------------------------------------------------------
# 404 — Not Found
# ---------------------------------------------------------------------------


class NVRNotFoundError(NVRServiceError):
    status_code = 404
    error_code = "NVR_NOT_FOUND"

    def __init__(self, nvr_id: str | None = None):
        detail = f"NVR '{nvr_id}' not found." if nvr_id else "NVR not found."
        super().__init__(detail)


class ChannelNotFoundError(NVRServiceError):
    status_code = 404
    error_code = "CHANNEL_NOT_FOUND"

    def __init__(self, channel_id: str | None = None):
        detail = (
            f"Channel '{channel_id}' not found."
            if channel_id
            else "Channel not found."
        )
        super().__init__(detail)


# ---------------------------------------------------------------------------
# 400 — Bad Request
# ---------------------------------------------------------------------------


class UnsupportedVendorError(NVRServiceError):
    status_code = 400
    error_code = "UNSUPPORTED_VENDOR"

    def __init__(self, vendor: str):
        super().__init__(
            f"Vendor '{vendor}' is not supported. "
            "Supported vendors: hikvision, dahua, generic."
        )


class InvalidConfigurationError(NVRServiceError):
    status_code = 400
    error_code = "INVALID_CONFIGURATION"

    def __init__(self, message: str = "Invalid NVR configuration."):
        super().__init__(message)


# ---------------------------------------------------------------------------
# 401 — Unauthorized
# ---------------------------------------------------------------------------


class InvalidCredentialsError(NVRServiceError):
    status_code = 401
    error_code = "INVALID_CREDENTIALS"

    def __init__(self):
        super().__init__(
            "RTSP authentication failed — check NVR username and password."
        )


# ---------------------------------------------------------------------------
# 403 — Forbidden
# ---------------------------------------------------------------------------


class StreamTokenError(NVRServiceError):
    status_code = 403
    error_code = "STREAM_TOKEN_INVALID"

    def __init__(self, message: str = "Stream access token is invalid or expired."):
        super().__init__(message)


# ---------------------------------------------------------------------------
# 502 — Bad Gateway (NVR unreachable)
# ---------------------------------------------------------------------------


class NVRUnreachableError(NVRServiceError):
    status_code = 502
    error_code = "NVR_UNREACHABLE"

    def __init__(self, ip_address: str | None = None):
        detail = (
            f"NVR at {ip_address} is unreachable."
            if ip_address
            else "NVR is unreachable."
        )
        super().__init__(detail)


# ---------------------------------------------------------------------------
# 422 — Validation Failure (RTSP check failed)
# ---------------------------------------------------------------------------


class RTSPValidationError(NVRServiceError):
    status_code = 422
    error_code = "RTSP_VALIDATION_FAILED"

    def __init__(self, message: str = "RTSP stream validation failed."):
        super().__init__(message)


# ---------------------------------------------------------------------------
# Global Exception Handler (registers with FastAPI)
# ---------------------------------------------------------------------------


async def nvr_exception_handler(request: Request, exc: NVRServiceError) -> JSONResponse:
    """
    Maps NVRServiceError subclasses to structured JSON error responses.

    Register in main.py via:
        app.add_exception_handler(NVRServiceError, nvr_exception_handler)
    """
    return JSONResponse(
        status_code=exc.status_code,
        content={
            "error": {
                "code": exc.error_code,
                "message": exc.message,
            }
        },
    )
