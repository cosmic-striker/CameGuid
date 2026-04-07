"""
Tests for the RTSP connection validation service.

Uses mocked subprocess calls to test FFprobe error classification
without requiring actual RTSP streams or FFmpeg installation.
"""

import asyncio
import json
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from app.services.validation_service import (
    _classify_ffprobe_error,
    _parse_ffprobe_output,
    validate_rtsp_connection,
)


class TestErrorClassification:
    """Tests for FFprobe error message classification."""

    def test_unauthorized(self):
        msg = _classify_ffprobe_error("Server returned 401 Unauthorized", 1)
        assert "Authentication failed" in msg

    def test_connection_refused(self):
        msg = _classify_ffprobe_error("Connection refused by server", 1)
        assert "Connection refused" in msg

    def test_no_route(self):
        msg = _classify_ffprobe_error("No route to host", 1)
        assert "No route to host" in msg

    def test_network_unreachable(self):
        msg = _classify_ffprobe_error("Network is unreachable", 1)
        assert "Network unreachable" in msg

    def test_timeout(self):
        msg = _classify_ffprobe_error("Connection timed out", 1)
        assert "timed out" in msg

    def test_not_found(self):
        msg = _classify_ffprobe_error("404 Not Found", 1)
        assert "not found" in msg.lower()

    def test_generic_error(self):
        msg = _classify_ffprobe_error("Some unknown error occurred", 42)
        assert "exit code 42" in msg

    def test_long_error_truncated(self):
        long_err = "x" * 500
        msg = _classify_ffprobe_error(long_err, 1)
        # Should be truncated to avoid huge error messages
        assert len(msg) < 300


class TestFFprobeOutputParsing:
    """Tests for parsing FFprobe JSON output."""

    def test_parse_video_stream(self):
        output = json.dumps({
            "streams": [
                {
                    "codec_type": "video",
                    "codec_name": "h264",
                    "width": 1920,
                    "height": 1080,
                    "r_frame_rate": "25/1",
                    "profile": "High",
                }
            ]
        })
        result = _parse_ffprobe_output(output)
        assert result is not None
        assert result["video_codec"] == "h264"
        assert result["video_width"] == 1920
        assert result["video_height"] == 1080
        assert result["video_fps"] == 25.0
        assert result["video_profile"] == "High"

    def test_parse_video_and_audio(self):
        output = json.dumps({
            "streams": [
                {
                    "codec_type": "video",
                    "codec_name": "h265",
                    "width": 3840,
                    "height": 2160,
                    "r_frame_rate": "30/1",
                },
                {
                    "codec_type": "audio",
                    "codec_name": "aac",
                    "sample_rate": "44100",
                },
            ]
        })
        result = _parse_ffprobe_output(output)
        assert result["video_codec"] == "h265"
        assert result["audio_codec"] == "aac"
        assert result["audio_sample_rate"] == "44100"

    def test_parse_empty_streams(self):
        output = json.dumps({"streams": []})
        assert _parse_ffprobe_output(output) is None

    def test_parse_invalid_json(self):
        assert _parse_ffprobe_output("not json") is None

    def test_parse_fractional_fps(self):
        output = json.dumps({
            "streams": [
                {
                    "codec_type": "video",
                    "codec_name": "h264",
                    "r_frame_rate": "30000/1001",
                }
            ]
        })
        result = _parse_ffprobe_output(output)
        assert result["video_fps"] == pytest.approx(29.97, abs=0.01)


class TestValidateRTSPConnection:
    """Tests for the full RTSP validation flow with mocked subprocess."""

    @pytest.mark.asyncio
    async def test_successful_validation(self):
        """A successful FFprobe run should return success with codec info."""
        ffprobe_output = json.dumps({
            "streams": [
                {
                    "codec_type": "video",
                    "codec_name": "h264",
                    "width": 1920,
                    "height": 1080,
                    "r_frame_rate": "25/1",
                }
            ]
        })

        mock_process = AsyncMock()
        mock_process.communicate = AsyncMock(
            return_value=(ffprobe_output.encode(), b"")
        )
        mock_process.returncode = 0

        with patch("app.services.validation_service.asyncio.create_subprocess_exec",
                   return_value=mock_process):
            result = await validate_rtsp_connection(
                "rtsp://admin:pass@192.168.1.100:554/stream1",
                timeout=5,
            )

        assert result.success is True
        assert result.latency_ms is not None
        assert result.latency_ms >= 0
        assert result.codec_info is not None
        assert result.codec_info["video_codec"] == "h264"

    @pytest.mark.asyncio
    async def test_failed_validation(self):
        """A failed FFprobe run should return failure with error message."""
        mock_process = AsyncMock()
        mock_process.communicate = AsyncMock(
            return_value=(b"", b"Connection refused")
        )
        mock_process.returncode = 1

        with patch("app.services.validation_service.asyncio.create_subprocess_exec",
                   return_value=mock_process):
            result = await validate_rtsp_connection(
                "rtsp://admin:pass@192.168.1.100:554/stream1",
                timeout=5,
            )

        assert result.success is False
        assert "Connection refused" in result.error
        assert result.latency_ms is not None

    @pytest.mark.asyncio
    async def test_auth_failure(self):
        """401 Unauthorized should be classified as auth failure."""
        mock_process = AsyncMock()
        mock_process.communicate = AsyncMock(
            return_value=(b"", b"Server returned 401 Unauthorized")
        )
        mock_process.returncode = 1

        with patch("app.services.validation_service.asyncio.create_subprocess_exec",
                   return_value=mock_process):
            result = await validate_rtsp_connection(
                "rtsp://admin:wrongpass@192.168.1.100:554/stream1",
                timeout=5,
            )

        assert result.success is False
        assert "Authentication failed" in result.error

    @pytest.mark.asyncio
    async def test_ffprobe_not_found(self):
        """Should handle missing FFprobe gracefully."""
        with patch(
            "app.services.validation_service.asyncio.create_subprocess_exec",
            side_effect=FileNotFoundError("ffprobe"),
        ):
            result = await validate_rtsp_connection(
                "rtsp://admin:pass@192.168.1.100:554/stream1",
                timeout=5,
            )

        assert result.success is False
        assert "ffprobe not found" in result.error

    @pytest.mark.asyncio
    async def test_timeout_handling(self):
        """Should handle subprocess timeout gracefully."""
        mock_process = AsyncMock()
        mock_process.communicate = AsyncMock(
            side_effect=asyncio.TimeoutError()
        )
        mock_process.kill = MagicMock()
        mock_process.wait = AsyncMock()

        with patch("app.services.validation_service.asyncio.create_subprocess_exec",
                   return_value=mock_process):
            result = await validate_rtsp_connection(
                "rtsp://admin:pass@192.168.1.100:554/stream1",
                timeout=1,
            )

        assert result.success is False
        assert "timed out" in result.error.lower()
