"""Create NVR tables

Revision ID: 001_create_nvr_tables
Revises: None
Create Date: 2026-04-07
"""

from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID

# revision identifiers, used by Alembic
revision: str = "001_create_nvr_tables"
down_revision: Union[str, None] = None
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    # Create vendor enum type
    vendor_enum = sa.Enum(
        "hikvision", "dahua", "generic",
        name="nvr_vendor_enum",
    )
    vendor_enum.create(op.get_bind(), checkfirst=True)

    # NVR Devices table
    op.create_table(
        "nvr_devices",
        sa.Column("id", UUID(as_uuid=True), primary_key=True),
        sa.Column("name", sa.String(128), nullable=False),
        sa.Column("ip_address", sa.String(45), nullable=False),
        sa.Column("rtsp_port", sa.Integer(), nullable=False, server_default="554"),
        sa.Column("onvif_port", sa.Integer(), nullable=False, server_default="80"),
        sa.Column("username", sa.String(128), nullable=False),
        sa.Column("password_encrypted", sa.Text(), nullable=False),
        sa.Column(
            "vendor",
            vendor_enum,
            nullable=False,
            server_default="generic",
        ),
        sa.Column("channel_count", sa.Integer(), nullable=False, server_default="4"),
        sa.Column(
            "created_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
        sa.Column(
            "updated_at",
            sa.DateTime(timezone=True),
            nullable=False,
            server_default=sa.text("now()"),
        ),
    )
    op.create_index("ix_nvr_devices_name", "nvr_devices", ["name"])
    op.create_index("ix_nvr_devices_ip", "nvr_devices", ["ip_address"])

    # NVR Channels table
    op.create_table(
        "nvr_channels",
        sa.Column("id", UUID(as_uuid=True), primary_key=True),
        sa.Column(
            "nvr_id",
            UUID(as_uuid=True),
            sa.ForeignKey("nvr_devices.id", ondelete="CASCADE"),
            nullable=False,
        ),
        sa.Column("channel_number", sa.Integer(), nullable=False),
        sa.Column("name", sa.String(128), nullable=True),
        sa.Column("stream_main_url", sa.String(512), nullable=True),
        sa.Column("stream_sub_url", sa.String(512), nullable=True),
        sa.Column("enabled", sa.Boolean(), nullable=False, server_default="true"),
        sa.UniqueConstraint("nvr_id", "channel_number", name="uq_nvr_channel"),
    )
    op.create_index("ix_nvr_channels_nvr_id", "nvr_channels", ["nvr_id"])


def downgrade() -> None:
    op.drop_table("nvr_channels")
    op.drop_table("nvr_devices")

    # Drop the enum type
    sa.Enum(name="nvr_vendor_enum").drop(op.get_bind(), checkfirst=True)
