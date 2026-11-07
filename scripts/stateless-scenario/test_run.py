import base64
import contextlib
import hashlib
import json
import io
import os
import tempfile
from types import SimpleNamespace
from unittest import mock
from pathlib import Path
import struct
import sys
import unittest
import urllib.parse

import run as scenario


class ScenarioChecks(unittest.TestCase):
    def test_hep_lengths_ids_and_timestamp(self):
        call_id = "scenario-unique@example.test"
        packet = scenario.hep_packet(call_id)
        self.assertEqual(packet[:4], b"HEP3")
        self.assertEqual(struct.unpack("!H", packet[4:6])[0], len(packet))
        offset, chunks = 6, {}
        while offset < len(packet):
            vendor, kind, size = struct.unpack("!HHH", packet[offset:offset + 6])
            self.assertEqual(vendor, 0)
            self.assertGreaterEqual(size, 6)
            self.assertLessEqual(offset + size, len(packet))
            chunks[kind] = packet[offset + 6:offset + size]
            offset += size
        self.assertEqual(offset, len(packet))
        self.assertEqual(chunks[17].decode(), call_id)
        self.assertIn(("Call-ID: " + call_id).encode(), chunks[15])
        self.assertGreater(struct.unpack("!I", chunks[9])[0], 1_700_000_000)

    def test_loss_duplicates_and_unknown_rows_fail(self):
        for counts in [{}, {"a": 2}, {"a": 1, "unknown": 1}]:
            with self.subTest(counts=counts), self.assertRaises(scenario.ScenarioError):
                scenario.check_rows(counts, {"a"}, {"a", "pending"})
        # HTTP-accepted but uncommitted rows may be absent after a crash.
        scenario.check_rows({"a": 1}, {"a"}, {"a", "pending"})
        scenario.check_rows({"a": 1, "pending": 1}, {"a"}, {"a", "pending"})

    def test_s3_endpoint_and_argument_validation(self):
        args = scenario.arguments(["--binary", sys.executable, "--bucket", "test-bucket",
                                   "--endpoint", "https://s3.example.test", "--region", "test-region"])
        self.assertEqual(args.region, "test-region")
        self.assertEqual(args.endpoint, "https://s3.example.test")
        self.assertFalse(args.no_faults)
        self.assertFalse(args.cleanup)
        self.assertEqual(args.binary, Path(sys.executable).resolve())

    def test_region_is_optional_with_environment_and_cli_overrides(self):
        base = ["--binary", sys.executable, "--bucket", "test-bucket",
                "--endpoint", "https://s3.example.test"]
        with mock.patch.dict(os.environ, {}, clear=True):
            self.assertEqual(scenario.arguments(base).region, "us-east-1")
            os.environ["AWS_DEFAULT_REGION"] = "default-region"
            self.assertEqual(scenario.arguments(base).region, "default-region")
            os.environ["AWS_REGION"] = "preferred-region"
            self.assertEqual(scenario.arguments(base).region, "preferred-region")
            self.assertEqual(scenario.arguments(base + ["--region", "cli-region"]).region, "cli-region")

    def test_dotenv_aws_values_and_override_precedence(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / ".env"
            path.write_text(
                "# synthetic test credentials\n"
                "export AWS_ACCESS_KEY_ID='file-key'\n"
                "AWS_SECRET_ACCESS_KEY='literal-${UNEXPANDED}#tail'\n"
                "AWS_ENDPOINT_URL_S3=https://file.s3.example.test\n"
                "AWS_DEFAULT_REGION=test-region\n"
            )
            with mock.patch.dict(os.environ, {"AWS_ACCESS_KEY_ID": "exported-key"}, clear=True):
                self.assertTrue(scenario.load_env_file(path))
                self.assertEqual(os.environ["AWS_ACCESS_KEY_ID"], "exported-key")
                self.assertEqual(os.environ["AWS_SECRET_ACCESS_KEY"], "literal-${UNEXPANDED}#tail")
                base = ["--binary", sys.executable, "--bucket", "test-bucket"]
                args = scenario.arguments(base)
                self.assertEqual(args.endpoint, "https://file.s3.example.test")
                self.assertEqual(args.region, "test-region")
                self.assertEqual(args.env_file, scenario.default_env_file())
                args = scenario.arguments(base + ["--endpoint", "https://override.example.test", "--region", "override"])
                self.assertEqual(args.endpoint, "https://override.example.test")
                self.assertEqual(args.region, "override")
                runner = object.__new__(scenario.Runner)
                runner.dsns = ["settings-dsn", "catalog-dsn"]
                runner.jwt_secret = "jwt"
                runner.password_hash = "hash"
                runner.node_token = "node"
                env = runner.child_env()
                for section in ("NODE", "STORAGE"):
                    self.assertEqual(env[f"HOMER_{section}_DUCKLAKE_S3_ACCESS_KEY_ID"], "exported-key")
                    self.assertEqual(env[f"HOMER_{section}_DUCKLAKE_S3_SECRET_ACCESS_KEY"], "literal-${UNEXPANDED}#tail")

    def test_grouped_env_file_precedes_legacy_file(self):
        with tempfile.TemporaryDirectory() as directory:
            scripts = Path(directory) / "scripts"
            group = scripts / "stateless-scenario"
            group.mkdir(parents=True)
            preferred, legacy = group / ".env", scripts / ".env"
            with mock.patch.object(scenario, "DEFAULT_ENV_PATH", preferred):
                self.assertEqual(scenario.default_env_file(), preferred)
                legacy.write_text("# synthetic legacy file\n")
                self.assertEqual(scenario.default_env_file(), legacy)
                preferred.write_text("# synthetic grouped file\n")
                self.assertEqual(scenario.default_env_file(), preferred)

    def test_absent_dotenv_keeps_exported_configuration(self):
        with tempfile.TemporaryDirectory() as directory:
            self.assertFalse(scenario.load_env_file(Path(directory) / "absent.env"))
        with mock.patch.dict(os.environ, {"AWS_REGION": "preferred", "AWS_DEFAULT_REGION": "fallback"}):
            args = scenario.arguments(["--binary", sys.executable, "--bucket", "test-bucket",
                                       "--endpoint", "https://s3.example.test"])
            self.assertEqual(args.region, "preferred")

    def test_rejects_unsafe_or_invalid_arguments(self):
        base = ["--binary", sys.executable, "--bucket", "test-bucket",
                                   "--endpoint", "https://s3.example.test", "--region", "test-region"]
        for extra in [["--rows", "17"], ["--prefix", "../outside"],
                      ["--endpoint", "https://user:secret@s3.example.test"],
                      ["--endpoint", "https://s3.example.test/path"],
                      ["--bucket", "gs://wrong-format"]]:
            with self.subTest(extra=extra), mock.patch("sys.stderr", new_callable=io.StringIO):
                with self.assertRaises(SystemExit):
                    scenario.arguments(base + extra)

    def test_cleanup_is_scoped_and_attempts_databases_after_cloud_failure(self):
        for page in [RuntimeError("cloud listing failed"),
                     [{"Contents": [{"Key": "someone-elses/object"}]}]]:
            runner = object.__new__(scenario.Runner)
            runner.args = SimpleNamespace(bucket="test-bucket")
            runner.prefix = "owned/run/"
            runner.admin_dsn = "test-dsn"
            runner.created = ["homer_scenario_own_db"]
            runner.report = {}
            runner.save = mock.Mock()
            runner.s3 = mock.Mock()
            listing = runner.s3.get_paginator.return_value.paginate
            if isinstance(page, Exception):
                listing.side_effect = page
            else:
                listing.return_value = page
            runner.pg = mock.MagicMock()
            connection = runner.pg.connect.return_value.__enter__.return_value
            sql = mock.Mock()
            with mock.patch.dict(sys.modules, {"psycopg": SimpleNamespace(sql=sql)}):
                with self.assertRaises(scenario.ScenarioError):
                    runner.cleanup()
            runner.s3.delete_object.assert_not_called()
            sql.Identifier.assert_called_once_with("homer_scenario_own_db")
            connection.execute.assert_called_once()
            self.assertEqual(runner.report["cleanup"], "failed")

    def test_schedule_fault_restores_schema_and_lock_on_timeout(self):
        runner = object.__new__(scenario.Runner)
        runner.release_maintenance = mock.Mock()
        runner.hold_maintenance = mock.Mock()
        runner.survivor_compacts = mock.Mock()
        runner.seed = mock.Mock()
        runner.file_count = mock.Mock(return_value=8)
        runner.sql = mock.Mock()
        runner.wait = mock.Mock(side_effect=scenario.ScenarioError("merge timed out"))
        runner.record = mock.Mock()
        with self.assertRaisesRegex(scenario.ScenarioError, "merge timed out"):
            runner.schedule_failure()
        statements = [call.args[0] for call in runner.sql.call_args_list]
        self.assertEqual(len(statements), 2)
        self.assertIn("completed_at TO scenario_broken", statements[0])
        self.assertIn("scenario_broken TO completed_at", statements[1])
        self.assertEqual(runner.hold_maintenance.call_count, 2)
        runner.record.assert_not_called()

    def test_verification_barrier_preserves_existing_lease_and_releases_on_error(self):
        for existing in (False, True):
            for fail in (False, True):
                with self.subTest(existing=existing, fail=fail):
                    runner = object.__new__(scenario.Runner)
                    lease = object()
                    runner.maintenance = lease if existing else None
                    events = []
                    def hold():
                        events.append("held")
                        runner.maintenance = lease
                    def release():
                        events.append("released")
                        runner.maintenance = None
                    runner.hold_maintenance = mock.Mock(side_effect=hold)
                    runner.release_maintenance = mock.Mock(side_effect=release)
                    with self.assertRaisesRegex(scenario.ScenarioError, "read failed") if fail else contextlib.nullcontext():
                        with runner.maintenance_idle():
                            self.assertIs(runner.maintenance, lease)
                            events.append("read")
                            if fail:
                                raise scenario.ScenarioError("read failed")
                    self.assertEqual(events, ["read"] if existing else ["held", "read", "released"])
                    self.assertIs(runner.maintenance, lease if existing else None)

    def test_failed_maintenance_wait_closes_unowned_connection(self):
        runner = object.__new__(scenario.Runner)
        runner.maintenance = None
        runner.args = SimpleNamespace(timeout=10)
        runner.dsns = ["settings", "catalog"]
        runner.pg = mock.Mock()
        connection = runner.pg.connect.return_value
        connection.execute.side_effect = [None, RuntimeError("wait timed out")]
        with self.assertRaisesRegex(RuntimeError, "wait timed out"):
            runner.hold_maintenance()
        self.assertIsNone(runner.maintenance)
        connection.close.assert_called_once()

    def test_oauth_stub_requires_valid_pkce(self):
        provider = scenario.OAuthProvider()
        try:
            verifier = "valid-pkce-verifier"
            provider.challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).decode().rstrip("=")
            def exchange(value, expected):
                form = urllib.parse.urlencode({"code": provider.code, "code_verifier": value}).encode()
                return scenario.http("POST", provider.url + "/token", form, expected=(expected,))[2]
            exchange("incorrect", 400)
            payload = json.loads(exchange(verifier, 200))
            self.assertEqual(payload["access_token"], provider.access)
        finally:
            provider.close()


if __name__ == "__main__":
    unittest.main()
