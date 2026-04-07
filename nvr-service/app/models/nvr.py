"""
NVRDevice ORM model.

Represents a Network Video Recorder device on the local network.
Passwords are stored encrypted using Fernet (see core.security).
"""

import uuid
from datetime import datetime

from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
    Enum,
    Integer,
    String,
    Text,
    func,
)
from sqlalchemy.dialects.postgresql import UUID
from sqlalchemy.orm import relationship

from app.core.database import Base

# Supported NVR vendors — add new entries here and create a matching
# vendor strategy in app/vendors/ to extend support.
VendorEnum = Enum(
    "hikvision",
    "dahua",
    "generic",
    name="nvr_vendor_enum",
    create_constraint=True,
)


class NVRDevice(Base):
    """
    A managed NVR device.

    Stores connection details and encrypted credentials. The `vendor`
    field selects the RTSP URL generation strategy at runtime.
    """

    __tablename__ = "nvr_devices"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        index=True,
    )
    name = Column(String(128), nullable=False, index=True)
    ip_address = Column(String(45), nullable=False)  # Supports IPv4 and IPv6
    rtsp_port = Column(Integer, nullable=False, default=554)
    onvif_port = Column(Integer, nullable=False, default=80)
    username = Column(String(128), nullable=False)
    password_encrypted = Column(Text, nullable=False)  # Fernet ciphertext
    vendor = Column(VendorEnum, nullable=False, default="generic")
    channel_count = Column(Integer, nullable=False, default=4)

    created_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
    )
    updated_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        onupdate=func.now(),
    )

    # Relationship to channels — cascade delete when NVR is removed
    channels = relationship(
        "NVRChannel",
        back_populates="nvr",
        cascade="all, delete-orphan",
        passive_deletes=True,
        order_by="NVRChannel.channel_number",
    )

    def __repr__(self) -> str:
        return (
            f"<NVRDevice id={self.id} name='{self.name}' "
            f"ip={self.ip_address} vendor={self.vendor}>"
        )
