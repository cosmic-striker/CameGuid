"""
Models package — exports all ORM models for Alembic and application use.
"""

from app.models.nvr import NVRDevice
from app.models.channel import NVRChannel

__all__ = ["NVRDevice", "NVRChannel"]
