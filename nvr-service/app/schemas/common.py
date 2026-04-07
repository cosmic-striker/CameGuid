"""
Common response schemas shared across all API endpoints.
"""

from pydantic import BaseModel


class ErrorDetail(BaseModel):
    """Structured error detail in API responses."""
    code: str
    message: str


class ErrorResponse(BaseModel):
    """Standard error response envelope."""
    error: ErrorDetail


class SuccessResponse(BaseModel):
    """Generic success response for operations without a specific payload."""
    message: str
    detail: dict | None = None


class HealthResponse(BaseModel):
    """Health check response."""
    status: str = "ok"
    service: str = "nvr-service"
