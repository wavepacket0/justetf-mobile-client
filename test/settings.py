import os
from dataclasses import dataclass, field
from pathlib import Path

from dotenv import dotenv_values


MODES = ("offline", "online-readonly", "online-mutations")
LIVE_BASE_URL = "https://api.mobile.justetf.com"
OFFLINE_EMAIL = "test@example.invalid"
OFFLINE_PASSWORD = "test:password"


@dataclass(frozen=True)
class Settings:
    mode: str
    email: str = field(repr=False)
    password: str = field(repr=False)


def load_settings(root: Path, environment=None, mode=None) -> Settings:
    values = dict(dotenv_values(root / ".env", interpolate=False))
    values.update(os.environ if environment is None else environment)
    selected = mode if mode is not None else values.get("JUSTETF_TEST_MODE", "offline")
    if selected not in MODES:
        raise ValueError("JUSTETF_TEST_MODE must be offline, online-readonly, or online-mutations")
    if selected == "offline":
        return Settings(selected, OFFLINE_EMAIL, OFFLINE_PASSWORD)
    email = values.get("JUSTETF_TEST_EMAIL", "") or ""
    password = values.get("JUSTETF_TEST_PASSWORD", "") or ""
    if not email.strip() or not password.strip():
        raise ValueError("online tests require JUSTETF_TEST_EMAIL and JUSTETF_TEST_PASSWORD")
    return Settings(selected, email, password)