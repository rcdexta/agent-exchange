"""Exercise the installer with a real binary and offline release downloads."""
import hashlib
import os
from pathlib import Path
import platform
import subprocess
import tarfile
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[1]
BINARY = ROOT / "dist/package/ax"
if not BINARY.exists():
    BINARY = ROOT / "bin/ax"
VERSION = subprocess.check_output([BINARY, "version"], text=True).strip()
OS = "darwin" if platform.system() == "Darwin" else "linux"
ARCH = "arm64" if platform.machine() in ("arm64", "aarch64") else "amd64"
ASSET = f"ax_{OS}_{ARCH}.tar.gz"


class InstallTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="ax-install-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.home = self.root / "home"
        self.home.mkdir(mode=0o700)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        self.release = self.root / "release"
        self.release.mkdir()
        with tarfile.open(self.release / ASSET, "w:gz") as archive:
            archive.add(BINARY, arcname="ax")
            archive.add(ROOT / "LICENSE", arcname="LICENSE")
        checksum = hashlib.sha256((self.release / ASSET).read_bytes()).hexdigest()
        (self.release / "checksums.txt").write_text(f"{checksum}  {ASSET}\n")
        curl = self.tools / "curl"
        curl.write_text('''#!/usr/bin/env python3
import os, pathlib, shutil, sys
args = sys.argv[1:]
base = "https://github.com/summationai/agent-exchange/releases"
version = "v" + os.environ["AX_TEST_VERSION"]
url = next(arg for arg in args if arg.startswith("https://"))
if url == base + "/latest":
    if os.environ.get("AX_VERSION"):
        sys.exit("pinned installs must not query latest")
    print(os.environ.get("AX_TEST_LATEST_URL", base + "/tag/" + version), end="")
elif url.startswith(base + "/download/" + version + "/"):
    if os.environ.get("AX_TEST_DOWNLOAD_FAIL"):
        sys.exit("release unavailable")
    dest = pathlib.Path(args[args.index("-o") + 1])
    shutil.copy(pathlib.Path(os.environ["AX_TEST_RELEASE"]) / url.rsplit("/", 1)[1], dest)
else:
    sys.exit("unexpected release URL: " + url)
''')
        curl.chmod(0o755)
        self.env = dict(os.environ, HOME=str(self.home), SHELL="/bin/zsh",
                        PATH=f"{self.tools}:{os.environ['PATH']}",
                        AX_TEST_VERSION=VERSION, AX_TEST_RELEASE=str(self.release))
        for key in ("AX_VERSION", "AX_INSTALL_DIR", "AX_NO_MODIFY_PATH", "ZDOTDIR"):
            self.env.pop(key, None)
        self.target = self.home / ".local/bin/ax"

    def install(self):
        return subprocess.run(["sh", ROOT / "install.sh"], env=self.env,
                              text=True, capture_output=True)

    def test_install_update_path_and_real_broker(self):
        profile = self.home / ".zshrc"
        profile.write_text("# existing shell settings\n")
        for _ in range(2):
            result = self.install()
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(subprocess.check_output([self.target, "version"], text=True).strip(), VERSION)
        self.assertEqual(profile.read_text().count('export PATH='), 1)
        self.assertTrue(profile.read_text().startswith("# existing shell settings\n"))
        state = self.root / "state"
        env = dict(self.env, AX_HOME=str(state))
        broker = subprocess.Popen([self.target, "serve"], env=env, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        try:
            for _ in range(100):
                if (state / "broker.sock").exists() or broker.poll() is not None:
                    break
                time.sleep(0.05)
            self.assertIsNone(broker.poll(), "installed broker failed to start")
            self.assertTrue((state / "broker.sock").exists())
            result = subprocess.run([self.target, "agents"], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
        finally:
            broker.terminate()
            broker.communicate(timeout=5)

    def test_failed_download_or_checksum_preserves_install(self):
        self.target.parent.mkdir(parents=True)
        self.target.write_text("old binary")
        for failure in ("download", "checksum"):
            with self.subTest(failure=failure):
                if failure == "download":
                    self.env["AX_TEST_DOWNLOAD_FAIL"] = "1"
                else:
                    self.env.pop("AX_TEST_DOWNLOAD_FAIL")
                    (self.release / "checksums.txt").write_text(f"{'0' * 64}  {ASSET}\n")
                self.assertNotEqual(self.install().returncode, 0)
                self.assertEqual(self.target.read_text(), "old binary")

    def test_custom_directory_does_not_modify_shell(self):
        self.env["AX_INSTALL_DIR"] = str(self.root / "custom")
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue((self.root / "custom/ax").exists())
        self.assertFalse((self.home / ".zshrc").exists())

    def test_pinned_release_skips_latest(self):
        self.env["AX_VERSION"] = "v" + VERSION
        result = self.install()
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_invalid_release_is_rejected(self):
        for tag in ("latest", "v1/../../other", "v1?query"):
            with self.subTest(tag=tag):
                self.env["AX_VERSION"] = tag
                self.assertNotEqual(self.install().returncode, 0)
                self.assertFalse(self.target.exists())
        self.env.pop("AX_VERSION")
        self.env["AX_TEST_LATEST_URL"] = "https://github.com/login"
        self.assertNotEqual(self.install().returncode, 0)
        self.assertFalse(self.target.exists())

    def test_symlink_target_is_preserved(self):
        other = self.root / "other"
        other.write_text("preserve me")
        self.target.parent.mkdir(parents=True)
        self.target.symlink_to(other)
        self.assertNotEqual(self.install().returncode, 0)
        self.assertTrue(self.target.is_symlink())
        self.assertEqual(other.read_text(), "preserve me")


if __name__ == "__main__":
    unittest.main()
