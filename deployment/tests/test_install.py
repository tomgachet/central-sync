"""Installer checks; all installation paths and host commands are sandboxed.

Run: python3 -m unittest discover -s deployment/tests -v
The sandbox copy bypasses root/OS checks only; it exercises real filesystem
operations without installing anything on the host or contacting Central.
"""

import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


DEPLOYMENT = Path(__file__).resolve().parents[1]


class InstallerTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="central-sync-installer-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bundle = self.root / "deployment"
        shutil.copytree(DEPLOYMENT, self.bundle)
        self.host = self.root / "host"
        for directory in ("etc/systemd/system", "etc/logrotate.d", "usr/local/bin",
                          "run/lock", "run/systemd/system"):
            (self.host / directory).mkdir(parents=True)
        (self.host / "etc/os-release").write_text('ID=debian\nVERSION_ID="13"\n')
        script = (self.bundle / "install.sh").read_text()
        script = script.replace('[[ $EUID -eq 0 ]]', 'true')
        for prefix in ("/etc/", "/var/", "/usr/local/", "/run/"):
            script = script.replace(prefix, str(self.host) + prefix)
        (self.bundle / "install.sh").write_text(script)
        self.bin = self.root / "commands"
        self.bin.mkdir()
        self.trace = self.root / "commands.trace"
        self.env = dict(os.environ, PATH=f"{self.bin}:{os.environ['PATH']}",
                        INSTALL_TRACE=str(self.trace), ACTIVE_UNITS="")
        self.write_command("getent", "#!/bin/sh\nexit 0\n")
        self.write_command("adduser", "#!/bin/sh\nexit 99\n")
        self.write_command("systemctl", '''#!/bin/sh
printf '%s\n' "$*" >> "$INSTALL_TRACE"
if [ "$1" = list-units ]; then printf '%s' "$ACTIVE_UNITS"; fi
''')
        for name in ("systemd-analyze", "logrotate"):
            self.write_command(name, "#!/bin/sh\nexit 0\n")
        # Keep real install operations and modes, omitting host account ownership.
        self.write_command("install", '''#!/usr/bin/env python3
import subprocess, sys
args = []
source = iter(sys.argv[1:])
for arg in source:
    if arg in ('-o', '-g'):
        next(source)
    else:
        args.append(arg)
sys.exit(subprocess.call(['/usr/bin/install', *args]))
''')
        self.binary = self.root / "binary"
        self.binary.write_text('#!/bin/sh\nprintf "central-sync test-version\\n"\n')
        self.script = self.bundle / "install.sh"

    def write_command(self, name, content):
        path = self.bin / name
        path.write_text(content)
        path.chmod(0o755)

    def run_installer(self, *args, success=True, original=False):
        script = DEPLOYMENT / "install.sh" if original else self.script
        result = subprocess.run(["bash", str(script), *args], env=self.env,
                                text=True, capture_output=True)
        if success:
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        return result

    def install_project(self):
        return self.run_installer("--project", "projet-1", "--binary", str(self.binary))

    def test_dry_run_and_input_validation_on_original_script(self):
        before = sorted(str(p) for p in self.root.rglob("*"))
        self.run_installer("--project", "projet-1", "--binary", str(self.binary),
                           "--dry-run", original=True)
        self.assertEqual(before, sorted(str(p) for p in self.root.rglob("*")))
        for args in (("--project", "../escape"), ("--project",), ("--unknown",),
                     ("--project", "projet-1", "--binary", "/missing/binary")):
            self.run_installer(*args, success=False, original=True)

    def test_initial_install_and_rerun_preserve_project_data(self):
        self.install_project()
        config = self.host / "etc/central-sync/projet-1"
        state = self.host / "var/lib/central-sync/projet-1"
        logs = self.host / "var/log/central-sync/projet-1"
        self.assertEqual((config / ".env").stat().st_mode & 0o777, 0o640)
        self.assertEqual(state.stat().st_mode & 0o777, 0o750)
        self.assertEqual((state / ".env").resolve(), config / ".env")
        self.assertTrue((state / "central-sync.log").is_symlink())
        preserved = {config / ".env": "SECRET=preserve\n",
                     config / "central_config.yaml": "custom config\n",
                     state / "central_session.json": "cached session\n",
                     logs / "central-sync.log": "existing logs\n"}
        for path, content in preserved.items():
            path.write_text(content)
        supplied = self.root / "replacement.env"
        supplied.write_text("SECRET=do-not-copy\n")
        self.run_installer("--project", "projet-1", "--env-file", str(supplied))
        for path, content in preserved.items():
            self.assertEqual(path.read_text(), content)
        commands = self.trace.read_text()
        self.assertNotIn("enable", commands)
        self.assertNotIn("start", commands)
        self.assertIn("daemon-reload", commands)

    def test_conflicting_path_is_rejected_before_installation(self):
        state = self.host / "var/lib/central-sync/projet-1"
        state.mkdir(parents=True)
        (state / ".env").write_text("do not overwrite\n")
        self.run_installer("--project", "projet-1", "--binary", str(self.binary),
                           success=False)
        self.assertFalse((self.host / "usr/local/bin/central-sync").exists())
        self.assertEqual((state / ".env").read_text(), "do not overwrite\n")

    def test_active_timer_blocks_binary_replacement(self):
        self.install_project()
        installed = self.host / "usr/local/bin/central-sync"
        before = installed.read_bytes()
        self.binary.write_text("replacement\n")
        self.env["ACTIVE_UNITS"] = "central-sync@projet-1.timer active waiting\n"
        self.run_installer("--project", "projet-1", "--binary", str(self.binary),
                           success=False)
        self.assertEqual(installed.read_bytes(), before)


if __name__ == "__main__":
    unittest.main()
