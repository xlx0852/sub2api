import hashlib
import importlib.util
import json
import tarfile
import tempfile
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("component_deploy.py")
SPEC = importlib.util.spec_from_file_location("component_deploy", MODULE_PATH)
component_deploy = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(component_deploy)


class BackendRuntimeRetirementTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name)
        frontend = self.root / "frontend" / "releases" / "1.3.0-live"
        frontend.mkdir(parents=True)
        (frontend / "sub2api-build.json").write_text(
            json.dumps({"frontend_version": "1.3.0", "min_backend_version": "1.0.0"})
        )
        (self.root / "frontend" / "current").symlink_to(frontend)

        current = self.root / "api" / "releases" / "1.4.0-live"
        current.mkdir(parents=True)
        (current / "sub2api").write_text("old")
        (self.root / "api" / "current").symlink_to(current)
        state = self.root / "state"
        state.mkdir()
        (state / ".codex-api-current").write_text(
            json.dumps({"version": "1.4.0", "min_frontend_version": "1.0.0"})
        )

    def tearDown(self):
        self.temp.cleanup()

    def backend_manifest(self):
        artifact = self.root / "backend.tar"
        binary = self.root / "candidate-sub2api"
        binary.write_text("new")
        with tarfile.open(artifact, "w") as archive:
            archive.add(binary, arcname="sub2api")
        return {
            "scope": "backend",
            "backend": {
                "version": "1.5.0",
                "min_frontend_version": "1.2.0",
                "artifact": str(artifact),
                "sha256": hashlib.sha256(artifact.read_bytes()).hexdigest(),
                "db_change": False,
            },
        }

    def test_deploy_and_rollback_retire_replaced_runtime(self):
        retired = self.root / "retired"
        retire_command = f"printf '%s\\n' {{release}} >> '{retired}'"

        component_deploy.install_backend(
            self.backend_manifest(), self.root, "true", "true", retire_command, None
        )
        self.assertIn("1.4.0-live", retired.read_text())
        self.assertEqual(
            "1.5.0",
            component_deploy.load_json(self.root / "state" / ".codex-api-current")["version"],
        )

        component_deploy.rollback(self.root, "backend", "true", retire_command)
        retired_releases = retired.read_text().splitlines()
        self.assertEqual(2, len(retired_releases))
        self.assertIn("1.5.0", retired_releases[1])
        self.assertEqual(
            "1.4.0",
            component_deploy.load_json(self.root / "state" / ".codex-api-current")["version"],
        )

    def test_backend_deploy_rejects_missing_retire_hook(self):
        with self.assertRaisesRegex(component_deploy.DeployError, "backend-retire-command"):
            component_deploy.install_backend(
                self.backend_manifest(), self.root, "true", "true", None, None
            )


if __name__ == "__main__":
    unittest.main()
