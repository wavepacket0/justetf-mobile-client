import tempfile
import unittest
from pathlib import Path

from settings import OFFLINE_EMAIL, OFFLINE_PASSWORD, load_settings


class SettingsTests(unittest.TestCase):
    def test_safe_default_and_explicit_modes(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.assertEqual(load_settings(root, {}).mode, "offline")
            for mode in ("online-readonly", "online-mutations"):
                settings = load_settings(root, {
                    "JUSTETF_TEST_MODE": mode,
                    "JUSTETF_TEST_EMAIL": "account@example.invalid",
                    "JUSTETF_TEST_PASSWORD": "synthetic-password",
                })
                self.assertEqual(settings.mode, mode)
                self.assertNotIn("synthetic-password", repr(settings))

    def test_dotenv_environment_precedence_and_literal_password(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / ".env").write_text(
                'JUSTETF_TEST_MODE=online-mutations\n'
                'JUSTETF_TEST_EMAIL=account@example.invalid\n'
                'JUSTETF_TEST_PASSWORD="literal-${NOT_INTERPOLATED}"\n', encoding="utf-8",
            )
            settings = load_settings(root, {"JUSTETF_TEST_MODE": "online-readonly"})
            self.assertEqual(settings.mode, "online-readonly")
            self.assertEqual(settings.password, "literal-${NOT_INTERPOLATED}")
            offline = load_settings(root, {}, mode="offline")
            self.assertEqual((offline.email, offline.password), (OFFLINE_EMAIL, OFFLINE_PASSWORD))

    def test_invalid_mode_and_missing_credentials_fail(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for mode in ("", "online", "typo", "online-readonly", "online-mutations"):
                with self.subTest(mode=mode), self.assertRaises(ValueError):
                    load_settings(root, {"JUSTETF_TEST_MODE": mode})