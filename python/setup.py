import os
import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

from setuptools import Distribution, setup
from setuptools.command.build_py import build_py
from wheel.bdist_wheel import bdist_wheel


REPOSITORY_ROOT = Path(__file__).resolve().parents[1]
PACKAGE_NAME = "justetf_mobile_client"


def native_library_name() -> str:
    if sys.platform == "win32":
        return "justetf-native.dll"
    if sys.platform == "darwin":
        return "libjustetf-native.dylib"
    return "libjustetf-native.so"


class BuildPyWithNativeLibrary(build_py):
    def run(self):
        super().run()

        library_name = native_library_name()
        package_output = Path(self.build_lib) / PACKAGE_NAME
        package_output.mkdir(parents=True, exist_ok=True)
        environment = os.environ.copy()
        environment.setdefault("CGO_ENABLED", "1")

        with tempfile.TemporaryDirectory(prefix="justetf-native-build-") as temporary:
            library_path = Path(temporary) / library_name
            subprocess.run(
                [
                    "go",
                    "build",
                    "-trimpath",
                    "-buildmode=c-shared",
                    "-o",
                    str(library_path),
                    "./cmd/justetf-native",
                ],
                cwd=REPOSITORY_ROOT,
                env=environment,
                check=True,
            )
            self.native_output = package_output / library_name
            shutil.copy2(library_path, self.native_output)

    def get_outputs(self, include_bytecode=1):
        outputs = super().get_outputs(include_bytecode)
        native_output = getattr(self, "native_output", None)
        if native_output is not None and native_output.is_file():
            outputs.append(str(native_output))
        return outputs


class NativeDistribution(Distribution):
    # Forces platlib layout so auditwheel accepts the bundled shared library.
    def has_ext_modules(self):
        return True


class PlatformWheel(bdist_wheel):
    def finalize_options(self):
        super().finalize_options()
        self.root_is_pure = False

    def get_tag(self):
        _, _, platform_tag = super().get_tag()
        return "py3", "none", platform_tag


setup(
    distclass=NativeDistribution,
    cmdclass={
        "build_py": BuildPyWithNativeLibrary,
        "bdist_wheel": PlatformWheel,
    }
)