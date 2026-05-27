"""
Vendor HTTP client for NVR auto-discovery.

NVR management APIs (Hikvision ISAPI, Dahua CGI) authenticate over HTTP
Basic or Digest auth. This module provides:

  - `get_with_auth()`: an async GET that first tries Basic auth and, on a
    Digest 401 challenge, transparently retries with a computed Digest
    Authorization header.

  - `build_digest_auth_header()`: a pure function that computes an RFC 7616
    (MD5 / qop="auth") Digest response. Kept separate so it can be unit-tested
    without any network access.
"""

import hashlib
import logging
import re
import secrets

import httpx

logger = logging.getLogger(__name__)

_qop_re = re.compile(r'([a-z]+)=\s*(?:"([^"]*)"|(\S+))', re.IGNORECASE)


# ---------------------------------------------------------------------------
# Digest auth (RFC 7616, MD5, qop="auth")
# ---------------------------------------------------------------------------


def _md5(value: str) -> str:
    return hashlib.md5(value.encode("utf-8")).hexdigest()


def parse_digest_challenge(www_authenticate: str) -> dict:
    """
    Parse a `WWW-Authenticate: Digest ...` challenge into a dict.

    Handles both quoted and unquoted parameter values.
    """
    params: dict[str, str] = {}
    for match in _qop_re.finditer(www_authenticate):
        quoted, unquoted = match.group(2), match.group(3)
        params[match.group(1)] = quoted if quoted is not None else unquoted
    return params


def build_digest_auth_header(
    method: str,
    uri: str,
    username: str,
    password: str,
    www_authenticate: str,
    nc: str = "00000001",
    cnonce: str | None = None,
) -> str | None:
    """
    Build a Digest `Authorization` header from a 401 challenge.

    Implements RFC 7616 with algorithm=MD5 and qop="auth". Returns None if the
    challenge is not a Digest challenge or is missing required fields.

    Args:
        method: HTTP method (e.g. "GET").
        uri: The request target (request-url or just path).
        username: NVR login username.
        password: NVR login password (plaintext).
        www_authenticate: The `WWW-Authenticate` response header value.
        nc: Nonce count ("00000001" for the first request).
        cnonce: Client nonce; a random one is generated if not provided.

    Returns:
        The full `Authorization` header value, or None if it cannot be built.
    """
    if not www_authenticate.lower().startswith("digest"):
        return None

    params = parse_digest_challenge(www_authenticate)
    realm = params.get("realm", "")
    nonce = params.get("nonce", "")
    opaque = params.get("opaque", "")
    qop = params.get("qop", "auth")

    if not realm or not nonce:
        logger.warning("Digest challenge missing realm/nonce — cannot authenticate.")
        return None

    if cnonce is None:
        cnonce = secrets.token_hex(8)

    # HA1 = MD5(username:realm:password)
    ha1 = _md5(f"{username}:{realm}:{password}")
    # HA2 = MD5(method:uri)
    ha2 = _md5(f"{method}:{uri}")
    # response = MD5(HA1:nonce:nc:cnonce:qop:HA2)
    response = _md5(f"{ha1}:{nonce}:{nc}:{cnonce}:{qop}:{ha2}")

    header_parts = [
        "Digest",
        f'username="{username}"',
        f'realm="{realm}"',
        f'nonce="{nonce}"',
        f'uri="{uri}"',
        f'qop={qop}',
        f"nc={nc}",
        f'cnonce="{cnonce}"',
        f'response="{response}"',
        "algorithm=MD5",
    ]
    if opaque:
        header_parts.append(f'opaque="{opaque}"')

    return ", ".join(header_parts)


# ---------------------------------------------------------------------------
# Authenticated GET
# ---------------------------------------------------------------------------


async def get_with_auth(
    url: str,
    username: str,
    password: str,
    timeout: int = 10,
) -> httpx.Response:
    """
    Perform an authenticated GET against an NVR management API.

    Tries Basic auth first. If the server responds 401 with a Digest
    challenge, the request is retried once with a computed Digest header.

    Args:
        url: Fully-qualified HTTP URL.
        username: NVR login username.
        password: NVR login password (plaintext).
        timeout: Request timeout in seconds.

    Returns:
        The final HTTP response.
    """
    basic = httpx.BasicAuth(username, password)

    async with httpx.AsyncClient(timeout=timeout, follow_redirects=False) as client:
        response = await client.get(url, auth=basic)

        if response.status_code != 401:
            return response

        challenge = response.headers.get("WWW-Authenticate", "")
        digest_header = build_digest_auth_header(
            "GET", url, username, password, challenge
        )
        if digest_header is None:
            logger.debug("No usable Digest challenge from %s — returning 401.", url)
            return response

        logger.debug(
            "Retrying %s with Digest auth (status was %s).",
            url,
            response.status_code,
        )
        response = await client.get(url, headers={"Authorization": digest_header})
        return response