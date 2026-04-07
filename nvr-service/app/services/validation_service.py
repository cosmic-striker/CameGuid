"""
RTSP connection validation service.

Uses FFprobe (from the FFmpeg suite) to test whether an RTSP stream
is reachable and returns codec/stream metadata. This avoids pulling
in heavyweight video processing libraries — FFprobe is a lightweight
probe tool that exits after analysing the stream headers.
"""

import asyncio
import json
import logging
import time

from app.core.config import get_settings
from app.schemas.nvr import ValidationResult

logger = logging.getLogger(__name__)


async def validate_rtsp_connection(
    rtsp_url: str,
    timeout: int | None = None,
) -> ValidationResult:
    """
    Test RTSP connectivity by probing the stream with FFprobe.

    Runs ffprobe as a subprocess with a timeout. Analyses the return code
    and output to determine connectivity, authentication, and codec info.

    Args:
        rtsp_url: Full RTSP URL including credentials.
        timeout: Probe timeout in seconds. Defaults to settings.FFMPEG_TIMEOUT.

    Returns:
        ValidationResult with success/failure, latency, and optional codec info.
    """
    settings = get_settings()
    if timeout is None:
        timeout = settings.FFMPEG_TIMEOUT

    # Build the FFprobe command
    # -v quiet: suppress noise
    # -print_format json: structured output
    # -show_streams: get codec info
    # -rtsp_transport tcp: more reliable than UDP for probing
    # -analyzeduration / -probesize: limit analysis time
    cmd = [
        "ffprobe",
        "-v", "quiet",
        "-print_format", "json",
        "-show_streams",
        "-rtsp_transport", "tcp",
        "-analyzeduration", str(timeout * 1_000_000),  # microseconds
        "-probesize", "500000",  # 500KB max probe
        "-i", rtsp_url,
    ]

    start_time = time.monotonic()

    try:
        process = await asyncio.create_subprocess_exec(
            *cmd,
            stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )

        try:
            stdout, stderr = await asyncio.wait_for(
                process.communicate(),
                timeout=timeout + 2,  # slight grace period
            )
        except asyncio.TimeoutError:
            process.kill()
            await process.wait()
            elapsed_ms = (time.monotonic() - start_time) * 1000
            logger.warning(
                "RTSP validation timed out after %.0fms for URL (redacted)",
                elapsed_ms,
            )
            return ValidationResult(
                success=False,
                latency_ms=round(elapsed_ms, 1),
                error=f"Connection timed out after {timeout} seconds.",
            )

        elapsed_ms = (time.monotonic() - start_time) * 1000

        if process.returncode != 0:
            stderr_text = stderr.decode("utf-8", errors="replace").strip()
            error_msg = _classify_ffprobe_error(stderr_text, process.returncode)
            logger.info(
                "RTSP validation failed (code=%d) in %.0fms: %s",
                process.returncode,
                elapsed_ms,
                error_msg,
            )
            return ValidationResult(
                success=False,
                latency_ms=round(elapsed_ms, 1),
                error=error_msg,
            )

        # Parse successful output
        codec_info = _parse_ffprobe_output(stdout.decode("utf-8", errors="replace"))

        logger.info("RTSP validation succeeded in %.0fms", elapsed_ms)
        return ValidationResult(
            success=True,
            latency_ms=round(elapsed_ms, 1),
            codec_info=codec_info,
        )

    except FileNotFoundError:
        logger.error("ffprobe not found — ensure FFmpeg is installed.")
        return ValidationResult(
            success=False,
            error="ffprobe not found. Ensure FFmpeg is installed on the server.",
        )
    except Exception as exc:
        elapsed_ms = (time.monotonic() - start_time) * 1000
        logger.exception("Unexpected error during RTSP validation")
        return ValidationResult(
            success=False,
            latency_ms=round(elapsed_ms, 1),
            error=f"Unexpected error: {str(exc)}",
        )


def _classify_ffprobe_error(stderr: str, return_code: int) -> str:
    """
    Classify the FFprobe error output into a user-friendly message.

    Common failure patterns:
    - "401 Unauthorized" → invalid credentials
    - "Connection refused" → NVR unreachable on that port
    - "No route to host" → network unreachable
    - "Server returned 4XX" → auth or config issue
    """
    stderr_lower = stderr.lower()

    if "401" in stderr_lower or "unauthorized" in stderr_lower:
        return "Authentication failed — invalid username or password."
    if "connection refused" in stderr_lower:
        return "Connection refused — NVR may be offline or RTSP port is wrong."
    if "no route to host" in stderr_lower:
        return "No route to host — NVR IP address may be incorrect."
    if "network is unreachable" in stderr_lower:
        return "Network unreachable — check network connectivity."
    if "timeout" in stderr_lower or "timed out" in stderr_lower:
        return "Connection timed out — NVR may be slow or unreachable."
    if "404" in stderr_lower or "not found" in stderr_lower:
        return "Stream not found — channel number or URL path may be incorrect."
    if "invalid data" in stderr_lower:
        return "Invalid stream data — the URL may point to a non-RTSP endpoint."

    # Fallback
    # Truncate long error messages
    short_stderr = stderr[:200] if len(stderr) > 200 else stderr
    return f"RTSP probe failed (exit code {return_code}): {short_stderr}"


def _parse_ffprobe_output(stdout: str) -> dict | None:
    """
    Parse the JSON output from FFprobe into a codec info summary.

    Returns a dict with keys like:
    - video_codec, video_width, video_height, video_fps
    - audio_codec
    """
    try:
        data = json.loads(stdout)
    except json.JSONDecodeError:
        return None

    streams = data.get("streams", [])
    if not streams:
        return None

    result = {}
    for stream in streams:
        codec_type = stream.get("codec_type", "")
        if codec_type == "video":
            result["video_codec"] = stream.get("codec_name", "unknown")
            result["video_width"] = stream.get("width")
            result["video_height"] = stream.get("height")
            # Parse frame rate from r_frame_rate (e.g. "25/1")
            r_frame_rate = stream.get("r_frame_rate", "")
            if "/" in r_frame_rate:
                num, den = r_frame_rate.split("/", 1)
                try:
                    result["video_fps"] = round(int(num) / int(den), 2)
                except (ValueError, ZeroDivisionError):
                    result["video_fps"] = None
            result["video_profile"] = stream.get("profile")
        elif codec_type == "audio":
            result["audio_codec"] = stream.get("codec_name", "unknown")
            result["audio_sample_rate"] = stream.get("sample_rate")

    return result if result else None
