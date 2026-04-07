"""
Vendor strategy registry.

Maps vendor names to their strategy implementations. To add a new
vendor, create a strategy class in this package and register it in
the VENDOR_STRATEGIES dict below.
"""

from app.vendors.base import VendorStrategy
from app.vendors.hikvision import HikvisionStrategy
from app.vendors.dahua import DahuaStrategy
from app.vendors.generic import GenericStrategy

# Registry mapping vendor name → strategy singleton
VENDOR_STRATEGIES: dict[str, VendorStrategy] = {
    "hikvision": HikvisionStrategy(),
    "dahua": DahuaStrategy(),
    "generic": GenericStrategy(),
}


def get_vendor_strategy(vendor: str) -> VendorStrategy:
    """
    Look up the vendor strategy by name.

    Args:
        vendor: One of "hikvision", "dahua", "generic".

    Returns:
        The corresponding VendorStrategy instance.

    Raises:
        KeyError: If the vendor is not registered.
    """
    strategy = VENDOR_STRATEGIES.get(vendor)
    if strategy is None:
        raise KeyError(
            f"No strategy registered for vendor '{vendor}'. "
            f"Available vendors: {', '.join(VENDOR_STRATEGIES.keys())}"
        )
    return strategy


__all__ = [
    "VendorStrategy",
    "HikvisionStrategy",
    "DahuaStrategy",
    "GenericStrategy",
    "VENDOR_STRATEGIES",
    "get_vendor_strategy",
]
