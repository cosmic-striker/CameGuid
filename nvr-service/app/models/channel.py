"""
NVRChannel ORM model.

Represents a single video channel on an NVR device. Channels are
auto-generated from the NVR's channel_count but can be individually
edited to override names or stream URLs.
"""

import uuid

from sqlalchemy import (
    Boolean,
    Column,
    ForeignKey,
    Integer,
    String,
    UniqueConstraint,
)
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import relationship

from app.core.database import Base


class NVRChannel(Base):
    """
    A video channel on an NVR.

    The stream_main_url and stream_sub_url are pre-computed when channels
    are generated. They can be manually overridden for non-standard setups.
    """

    __tablename__ = "nvr_channels"
    __table_args__ = (
        UniqueConstraint("nvr_id", "channel_number", name="uq_nvr_channel"),
    )

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        index=True,
    )
    nvr_id = Column(
        UUID(as_uuid=True),
        ForeignKey("nvr_devices.id", ondelete="CASCADE"),
        nullable=False,
        index=True,
    )
    channel_number = Column(Integer, nullable=False)
    name = Column(String(128), nullable=True)
    stream_main_url = Column(String(512), nullable=True)  # Generated / overridden
    stream_sub_url = Column(String(512), nullable=True)   # Generated / overridden
    enabled = Column(Boolean, nullable=False, default=True)

    # Back-reference to parent NVR
    nvr = relationship("NVRDevice", back_populates="channels")

    def __repr__(self) -> str:
        return (
            f"<NVRChannel id={self.id} nvr_id={self.nvr_id} "
            f"ch={self.channel_number} name='{self.name}'>"
        )
