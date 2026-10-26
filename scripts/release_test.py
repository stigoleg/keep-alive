"""Exercise release publishing with isolated GitHub/signing/build commands."""

import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().with_name("release.sh")
STUB = r'''#!/usr/bin/env python3
import json
import os
from pathlib import Path
import sys

command = Path(sys.argv[0]).name
args = sys.argv[1:]
with Path(os.environ["RELEASE_TEST_LOG"]).open("a") as log:
    log.write(json.dumps({"command": command, "args": args,
        "release_token": os.environ.get("GITHUB_TOKEN"),
        "package_token": os.environ.get("GH_PAT"),
        "api_token": os.environ.get("GH_TOKEN"),
        "notarize": os.environ.get("KEEPALIVE_NOTARIZE")}) + "\n")
if command == "uname":
    print("Darwin")
elif command == "git":
    if args == ["rev-parse", "--show-toplevel"]:
        print(os.environ["RELEASE_TEST_ROOT"])
    elif args == ["rev-parse", "HEAD"]:
        print("a" * 40)
    elif args[0] == "describe":
        print("v2.0.1")
    elif args[0] == "ls-remote":
        print("a" * 40 + "\trefs/tags/v2.0.1^{}")
    elif args != ["status", "--porcelain"]:
        sys.exit("unexpected git command")
elif command == "gh":
    if args == ["auth", "token"]:
        print("test-cli-token")
    elif args[0] == "api":
        print("false" if args[1] == os.environ.get("RELEASE_TEST_DENY") else "true")
    else:
        sys.exit("unexpected GitHub command")
elif command == "security":
    print('"Developer ID Application: Stig-Ole Gundersen (49P95434EE)"')
elif command == "goreleaser" and "--snapshot" in args:
    Path("dist").mkdir(exist_ok=True)
    Path("dist/artifacts.json").write_text(json.dumps([
        {"name": "keepalive_2.0.1_darwin_universal.tar.gz", "type": "Archive"}]))
'''


class ReleasePublishingTests(unittest.TestCase):
    def run_release(self, **overrides):
        with tempfile.TemporaryDirectory(prefix="keepalive-release-test-") as work:
            root = Path(work)
            tools = root / "tools"
            tools.mkdir()
            for name in ("git", "gh", "uname", "security", "xcrun", "goreleaser"):
                command = tools / name
                command.write_text(STUB)
                command.chmod(0o755)
            (root / "CHANGELOG.md").write_text("## [2.0.1] - 2026-10-27\n\nRelease fixes.\n")
            env = os.environ.copy()
            for name in list(env):
                if name in ("GITHUB_TOKEN", "GH_TOKEN", "GH_PAT", "PUBLISH_PACKAGE_MANAGERS") or name.startswith("KEEPALIVE_"):
                    del env[name]
            env.update(PATH=str(tools) + os.pathsep + env["PATH"],
                       RELEASE_TEST_ROOT=str(root), RELEASE_TEST_LOG=str(root / "calls.jsonl"))
            env.update(overrides)
            result = subprocess.run(["sh", str(SCRIPT)], cwd=root, env=env,
                                    input="y\n", capture_output=True, text=True, timeout=15)
            calls = [json.loads(line) for line in (root / "calls.jsonl").read_text().splitlines()]
            return result, calls

    def test_default_release_publishes_both_managers_with_cli_credentials(self):
        result, calls = self.run_release()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        builds = [call for call in calls if call["command"] == "goreleaser"]
        self.assertEqual(len(builds), 2)
        self.assertIn("--snapshot", builds[0]["args"])
        self.assertNotIn("--snapshot", builds[1]["args"])
        self.assertEqual(builds[1]["release_token"], "test-cli-token")
        self.assertEqual(builds[1]["package_token"], "test-cli-token")
        self.assertEqual(builds[1]["notarize"], "1")
        self.assertIn("Homebrew: Casks/keepalive.rb pushed", result.stdout)
        self.assertIn("Scoop:    keepalive.json pushed", result.stdout)

    def test_separate_package_token_is_used_to_check_repository_access(self):
        result, calls = self.run_release(GITHUB_TOKEN="test-release-token", GH_PAT="test-package-token")
        self.assertEqual(result.returncode, 0, result.stderr)
        checks = [call for call in calls if call["command"] == "gh"]
        self.assertEqual([call["args"][1] for call in checks],
                         ["repos/stigoleg/homebrew-tap", "repos/stigoleg/scoop-bucket"])
        self.assertTrue(all(call["api_token"] == "test-package-token" for call in checks))

    def test_missing_bucket_write_access_stops_before_building_or_publishing(self):
        result, calls = self.run_release(RELEASE_TEST_DENY="repos/stigoleg/scoop-bucket")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("token cannot push to stigoleg/scoop-bucket", result.stderr)
        self.assertFalse(any(call["command"] == "goreleaser" for call in calls))


if __name__ == "__main__":
    unittest.main()
