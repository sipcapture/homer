#!/usr/bin/env python3
"""Run two real Homer processes against PostgreSQL and S3-compatible storage.

Uses an existing bucket, two newly created private databases, and a unique object
prefix. HTTP acceptance is not treated as durable commit. See scripts/stateless-scenario/README.md.
"""
from __future__ import annotations

import argparse
import base64
import contextlib
import datetime as dt
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
import re
from pathlib import Path
import secrets
import signal
import socket
import struct
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

MAINTENANCE_KEY = 724193509
SCHEDULE_KEY = "homer.maintenance.compaction.completed_at"


class ScenarioError(RuntimeError):
    pass


def hep_packet(call_id: str) -> bytes:
    """One valid HEPv3 SIP INVITE, with a unique Call-ID and current timestamp."""
    now = time.time()
    sip = (
        "INVITE sip:bob@example.test SIP/2.0\r\n"
        "Via: SIP/2.0/UDP 192.0.2.1:5060;branch=z9hG4bKscenario\r\n"
        "From: Alice <sip:alice@example.test>;tag=scenario\r\n"
        "To: Bob <sip:bob@example.test>\r\n"
        f"Call-ID: {call_id}\r\nCSeq: 1 INVITE\r\n"
        "Contact: <sip:alice@192.0.2.1>\r\nContent-Length: 0\r\n\r\n"
    ).encode()
    chunks = []
    for kind, value in [
        (1, b"\x02"), (2, b"\x11"),
        (3, socket.inet_aton("192.0.2.1")), (4, socket.inet_aton("192.0.2.2")),
        (7, struct.pack("!H", 5060)), (8, struct.pack("!H", 5060)),
        (9, struct.pack("!I", int(now))),
        (10, struct.pack("!I", int((now % 1) * 1_000_000))),
        (11, b"\x01"), (12, struct.pack("!I", 2001)),
        (15, sip), (17, call_id.encode()),
    ]:
        chunks.append(struct.pack("!HHH", 0, kind, len(value) + 6) + value)
    body = b"".join(chunks)
    return b"HEP3" + struct.pack("!H", len(body) + 6) + body


def check_rows(counts: dict[str, int], confirmed: set[str], accepted: set[str]) -> None:
    missing = confirmed - counts.keys()
    extra = counts.keys() - accepted
    duplicates = {key: value for key, value in counts.items() if value != 1}
    if missing or extra or duplicates:
        raise ScenarioError(
            f"row integrity failed: missing_committed={sorted(missing)}, "
            f"unexpected={sorted(extra)}, duplicates={duplicates}"
        )


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def http(method, url, body=None, token=None, expected=(200,), timeout=15):
    headers = {}
    if isinstance(body, (dict, list)):
        body = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
    elif isinstance(body, bytes):
        headers["Content-Type"] = "application/octet-stream"
    if token:
        headers["Authorization"] = "Bearer " + token
    request = urllib.request.Request(url, body, headers, method=method)
    try:
        response = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect).open(request, timeout=timeout)
    except urllib.error.HTTPError as exc:
        response = exc
    with response:
        payload = response.read()
        if response.code not in expected:
            raise ScenarioError(f"{method} {urllib.parse.urlsplit(url).path}: HTTP {response.code}")
        return response.code, response.headers, payload


class OAuthProvider:
    """Local PKCE-validating provider; real external IdP availability is not tested."""
    def __init__(self):
        self.challenge = ""
        self.code = secrets.token_urlsafe(24)
        self.access = secrets.token_urlsafe(24)
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *_):
                pass

            def respond(self, code, payload):
                data = json.dumps(payload).encode()
                self.send_response(code)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_POST(self):
                form = urllib.parse.parse_qs(self.rfile.read(int(self.headers.get("Content-Length", 0))).decode())
                verifier = form.get("code_verifier", [""])[0]
                challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).decode().rstrip("=")
                if (self.path != "/token" or form.get("code", [""])[0] != owner.code
                        or not owner.challenge or challenge != owner.challenge):
                    return self.respond(400, {"error": "invalid_grant"})
                self.respond(200, {"access_token": owner.access, "token_type": "Bearer", "expires_in": 3600})

            def do_GET(self):
                if self.path != "/profile" or self.headers.get("Authorization") != "Bearer " + owner.access:
                    return self.respond(401, {})
                self.respond(200, {"sub": "scenario-user", "preferred_username": "scenario-user",
                                   "email": "scenario@example.test"})

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.url = f"http://127.0.0.1:{self.server.server_port}"
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


class Replica:
    def __init__(self, runner, name):
        self.runner, self.name = runner, name
        self.generation, self.process, self.log_handle = 0, None, None
        self.paused = False
        self.api = runner.port()
        self.ingest = runner.port()
        self.flight = runner.port(pair=True)

    @property
    def base(self):
        return f"http://127.0.0.1:{self.api}"

    def start(self):
        self.generation += 1
        folder = self.runner.output / self.name
        folder.mkdir(mode=0o700, exist_ok=True)
        lake = {
            "catalog_type": "postgres", "lake_name": "homer_lake",
            "data_path": self.runner.data_path, "batch_size": 8, "flush_interval_sec": 1,
            "data_inlining_row_limit": 0, "search_buffer": False, "shard_count": 1,
            "flush_queue": self.name == "b",
            "s3": {"endpoint": urllib.parse.urlsplit(self.runner.args.endpoint).netloc,
                   "region": self.runner.args.region,
                   "use_ssl": self.runner.args.endpoint.startswith("https://"), "url_style": "path"},
            "tuning": {"threads": 1, "memory_limit": self.runner.args.memory_limit,
                       "temp_directory": str(folder / f"spill-{self.generation}")},
            "compaction": {"enable": True, "engine": "duckdb", "check_interval_sec": 2,
                           "retention_days": 0, "snapshot_expire_interval_sec": 3600},
        }
        config = {
            "stateless": True,
            "ingest": {"enable": True, "worker_count": 1, "hep_stream": {"enable": False},
                       "udp": {"enable": False}, "tcp": {"enable": False}, "tls": {"enable": False},
                       "http": {"enable": True, "host": "127.0.0.1", "port": self.ingest}},
            "storage": {"enable": True, "ducklake": lake},
            "node": {"enable": True, "ducklake": lake,
                     "flight_server": {"host": "127.0.0.1", "port": self.flight}},
            "coordinator": {
                "enable": True, "lake_name": "homer_lake", "settings_db_type": "postgres",
                "http_server": {"host": "127.0.0.1", "port": self.api},
                "hep_stream": {"enable": False}, "jwt": {"cookie_enable": False},
                "auth": {"type": "internal", "admin_user": "scenario-admin"},
                "correlation": {"enable": False, "seed_default": False},
                "oauth2_provider": {
                    "enable": True, "name": "scenario", "client_id": "scenario", "use_pkce": True,
                    "auth_url": self.runner.provider.url + "/authorize",
                    "token_url": self.runner.provider.url + "/token",
                    "profile_url": self.runner.provider.url + "/profile",
                    "redirect_url": self.base + "/api/v4/auth/oauth2/scenario/callback",
                    "callback_url": "/oauth-complete",
                },
            },
            "prometheus": {"enable": False}, "log": {"output": ["stdout"], "level": "info"},
        }
        path = folder / "config.json"
        path.write_text(json.dumps(config, indent=2))
        path.chmod(0o600)
        self.log_path = folder / f"process-{self.generation}.log"
        self.log_handle = self.log_path.open("wb")
        self.log_path.chmod(0o600)
        self.process = subprocess.Popen(
            [str(self.runner.args.binary), "--config-path", str(path),
             "--pid-file", str(folder / "process.pid"), "--syslog-disable"],
            cwd=folder, env=self.runner.child_env(), stdout=self.log_handle,
            stderr=subprocess.STDOUT, start_new_session=True,
        )
        self.runner.report["processes"].append({
            "replica": self.name, "pid": self.process.pid, "generation": self.generation,
            "log": str(self.log_path.relative_to(self.runner.output)),
        })
        self.runner.wait(f"replica {self.name} ready", self.ready)

    def ready(self):
        if self.process.poll() is not None:
            raise ScenarioError(f"replica {self.name} exited with {self.process.returncode}; see its log")
        try:
            http("GET", self.base + "/health", timeout=1)
            http("GET", f"http://127.0.0.1:{self.ingest}/health", timeout=1)
            http("GET", f"http://127.0.0.1:{self.flight + 1}/health", timeout=1)
            return True
        except (OSError, ScenarioError):
            return False

    def pause(self):
        os.kill(self.process.pid, signal.SIGSTOP)
        self.paused = True

    def resume(self):
        if self.paused and self.process.poll() is None:
            os.kill(self.process.pid, signal.SIGCONT)
        self.paused = False

    def stop(self, crash=False):
        if self.process is None:
            return
        self.resume()
        if self.process.poll() is None:
            self.process.kill() if crash else self.process.terminate()
            try:
                self.process.wait(timeout=35)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait(timeout=10)
        if self.log_handle:
            self.log_handle.close()
        self.runner.save()


class Runner:
    def __init__(self, args):
        import bcrypt
        import boto3
        import psycopg
        from botocore.config import Config

        self.pg = psycopg
        self.args = args
        self.run_id = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-" + uuid.uuid4().hex[:8]
        self.output = (args.output / self.run_id).resolve()
        self.output.mkdir(parents=True, mode=0o700)
        self.output.chmod(0o700)
        self.prefix = args.prefix.strip("/") + "/" + self.run_id + "/"
        self.data_path = f"s3://{args.bucket}/{self.prefix}parquet"
        self.database_names = ["homer_scenario_" + self.run_id.lower().replace("-", "_") + "_" + suffix
                               for suffix in ("settings", "lake")]
        pg_options = psycopg.conninfo.conninfo_to_dict(os.environ["HOMER_SCENARIO_POSTGRES_DSN"])
        pg_options.setdefault("connect_timeout", "10")
        self.admin_dsn = psycopg.conninfo.make_conninfo(**pg_options)
        self.dsns = [psycopg.conninfo.make_conninfo(self.admin_dsn, dbname=name) for name in self.database_names]
        self.password = secrets.token_urlsafe(24)
        self.jwt_secret = secrets.token_urlsafe(32)
        self.node_token = secrets.token_urlsafe(32)
        self.password_hash = bcrypt.hashpw(self.password.encode(), bcrypt.gensalt()).decode()
        self.s3 = boto3.client(
            "s3", endpoint_url=args.endpoint, region_name=args.region,
            aws_access_key_id=os.environ["AWS_ACCESS_KEY_ID"],
            aws_secret_access_key=os.environ["AWS_SECRET_ACCESS_KEY"],
            config=Config(signature_version="s3v4", s3={"addressing_style": "path"},
                          request_checksum_calculation="when_required",
                          response_checksum_validation="when_required",
                          connect_timeout=10, read_timeout=30, retries={"max_attempts": 2}),
        )
        self.created = []
        self.ports = set()
        self.accepted, self.confirmed, self.uncertain = set(), set(), set()
        self.provider = None
        self.a = self.b = None
        self.maintenance = None
        binary_digest = hashlib.sha256()
        with args.binary.open("rb") as binary:
            for block in iter(lambda: binary.read(1024 * 1024), b""):
                binary_digest.update(block)
        binary_hash = binary_digest.hexdigest()
        self.report = {
            "run_id": self.run_id, "status": "running", "endpoint": args.endpoint,
            "region": args.region, "rows_per_phase": args.rows,
            "faults_requested": not args.no_faults, "timeout_seconds": args.timeout,
            "bucket": args.bucket, "prefix": self.prefix, "databases": self.database_names,
            "binary_sha256": binary_hash,
            "azure_maintenance": "untested against real Azure; this harness targets S3-compatible storage",
            "query_maintenance_coordination": "test-only wait for maintenance completion before durable-data verification; does not validate reads racing physical cleanup",
            "oauth_provider": "local PKCE-validating stub", "phases": [], "processes": [],
            "durability_contract": "HTTP acknowledgement is not a durable commit; confirmed rows must survive",
        }
        self.save()

    def save(self):
        self.report["created_databases"] = list(self.created)
        self.report["accepted_count"] = len(self.accepted)
        self.report["confirmed_ids"] = sorted(self.confirmed)
        self.report["uncommitted_at_flush_crash"] = sorted(self.uncertain)
        (self.output / "report.json").write_text(json.dumps(self.report, indent=2, default=str) + "\n")
        (self.output / "report.json").chmod(0o600)

    def wait(self, name, probe, timeout=None):
        deadline = time.monotonic() + (timeout or self.args.timeout)
        while time.monotonic() < deadline:
            result = probe()
            if result:
                return result
            time.sleep(0.1)
        raise ScenarioError("timed out: " + name)

    def port(self, pair=False):
        for _ in range(100):
            with socket.socket() as first:
                first.bind(("127.0.0.1", 0))
                number = first.getsockname()[1]
                if number in self.ports or (pair and (number + 1 in self.ports or number == 65535)):
                    continue
                if pair:
                    with socket.socket() as second:
                        try:
                            second.bind(("127.0.0.1", number + 1))
                        except OSError:
                            continue
                self.ports.update([number, number + 1] if pair else [number])
                return number
        raise ScenarioError("could not allocate local ports")

    def child_env(self):
        env = {key: value for key, value in os.environ.items() if not key.startswith("HOMER_")}
        env.update({
            "HOMER_COORDINATOR_SETTINGS_DB_PATH": self.dsns[0],
            "HOMER_STORAGE_DUCKLAKE_CATALOG_PATH": self.dsns[1],
            "HOMER_NODE_DUCKLAKE_CATALOG_PATH": self.dsns[1],
            "HOMER_COORDINATOR_JWT_SECRET": self.jwt_secret,
            "HOMER_COORDINATOR_AUTH_ADMIN_PASSWORD_HASH": self.password_hash,
            "HOMER_NODE_FLIGHT_SERVER_AUTH_TOKEN": self.node_token,
        })
        for section in ("STORAGE", "NODE"):
            for key in ("ACCESS_KEY_ID", "SECRET_ACCESS_KEY"):
                env[f"HOMER_{section}_DUCKLAKE_S3_{key}"] = os.environ[f"AWS_{key}"]
        return env

    def sql(self, statement, params=()):
        with self.pg.connect(self.dsns[1], autocommit=True) as connection:
            connection.execute("SET statement_timeout = '10s'")
            cursor = connection.execute(statement, params)
            return cursor.fetchall() if cursor.description else []

    def objects(self):
        pages = self.s3.get_paginator("list_objects_v2").paginate(Bucket=self.args.bucket, Prefix=self.prefix)
        return {item["Key"] for page in pages for item in page.get("Contents", [])
                if item["Key"].endswith(".parquet")}

    def record(self, name, **details):
        print(f"PASS {name}", flush=True)
        self.report["phases"].append({"name": name, "status": "passed",
                                      "completed_at": dt.datetime.now(dt.timezone.utc).isoformat(), **details})
        self.save()

    def provision(self):
        from psycopg import sql
        with self.pg.connect(self.admin_dsn, autocommit=True) as connection:
            for name in self.database_names:
                connection.execute(sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name)))
                self.created.append(name)
        probe = self.prefix + "credential-probe"
        self.s3.put_object(Bucket=self.args.bucket, Key=probe, Body=b"homer-scenario")
        response = self.s3.get_object(Bucket=self.args.bucket, Key=probe)
        try:
            if response["Body"].read() != b"homer-scenario":
                raise ScenarioError("object-store readback mismatch")
        finally:
            response["Body"].close()
        self.objects()
        self.s3.delete_object(Bucket=self.args.bucket, Key=probe)
        self.record("PostgreSQL provisioning and object PUT/GET/LIST/DELETE")

    def hold_maintenance(self):
        if self.maintenance is not None:
            return
        connection = self.pg.connect(self.dsns[1], autocommit=True)
        try:
            # Bound the test-only wait using the configured observation timeout.
            connection.execute("SELECT set_config('statement_timeout', %s, false)",
                               (str(max(1, int(self.args.timeout * 1000))),))
            connection.execute("SELECT pg_advisory_lock(%s)", (MAINTENANCE_KEY,))
        except BaseException:
            connection.close()
            raise
        self.maintenance = connection

    def release_maintenance(self):
        if self.maintenance is not None:
            self.maintenance.close()
            self.maintenance = None

    def login(self, replica):
        _, _, body = http("POST", replica.base + "/api/v4/auth/sessions",
                          {"username": "scenario-admin", "password": self.password}, expected=(201,))
        token = json.loads(body)["data"]["token"]
        if not token:
            raise ScenarioError("login did not return a JWT")
        return token

    @contextlib.contextmanager
    def maintenance_idle(self):
        """Test-only barrier: delay verification until physical cleanup finishes."""
        already_held = self.maintenance is not None
        if not already_held:
            self.hold_maintenance()
        try:
            yield
        finally:
            if not already_held:
                self.release_maintenance()

    def counts(self):
        with self.maintenance_idle():
            return self._counts()

    def _counts(self):
        query = "SELECT cid, count(*) AS n FROM homer_lake.hep_proto_1_call GROUP BY cid"
        _, _, body = http("POST", f"http://127.0.0.1:{self.b.flight + 1}/query",
                          {"sql": query}, self.node_token, expected=(200, 500, 503), timeout=self.args.timeout)
        result = json.loads(body)
        if not result.get("success"):
            error = str(result.get("error"))
            if not self.confirmed and "does not exist" in error:
                return {}
            raise ScenarioError("node query failed: " + error)
        counts = {row["cid"]: int(row["n"]) for row in result.get("data", [])}
        check_rows(counts, self.confirmed, self.accepted)
        self.confirmed.update(counts)
        return counts

    def ingest(self, replica, phase, count):
        ids = [f"{self.run_id}-{phase}-{i:05d}@scenario.test" for i in range(count)]
        for call_id in ids:
            # A failed HTTP response has an uncertain outcome too; stop instead
            # of retrying and potentially sending the same packet twice.
            self.accepted.add(call_id)
            http("POST", f"http://127.0.0.1:{replica.ingest}/api/hep", hep_packet(call_id))
        return set(ids)

    def seed(self, phase, replica=None):
        for group in range(8):
            ids = self.ingest(replica or self.a, f"{phase}-{group}", self.args.rows // 8)
            self.wait("seed rows committed", lambda: ids <= self.counts().keys())
        self.save()

    def search(self, token):
        with self.maintenance_idle():
            return self._search(token)

    def _search(self, token):
        now = time.time()
        _, _, body = http("POST", self.b.base + "/api/v4/transactions/search", {
            "filter": {"proto_type": 1, "event_type": "call"}, "param": {"limit": 50000},
            "timestamp": {"from": int((now - 3600) * 1000), "to": int((now + 60) * 1000)},
        }, token, timeout=self.args.timeout)
        rows = json.loads(body)["data"]["items"]
        counts = {}
        for row in rows:
            counts[row["cid"]] = counts.get(row["cid"], 0) + 1
        check_rows(counts, self.confirmed, self.accepted)
        self.confirmed.update(counts)
        return len(rows)

    def auth_and_exports(self):
        token = self.login(self.a)
        http("GET", self.b.base + "/api/v4/me", token=token)
        _, _, body = http("POST", self.a.base + "/api/v4/exports",
                          {"type": "text", "query": {"callid": self.run_id}}, token, expected=(201,))
        self.export_id = json.loads(body)["data"]["export_id"]
        self.export_check(token)
        http("DELETE", self.b.base + "/api/v4/auth/sessions/current", token=token, expected=(204,))
        http("GET", self.a.base + "/api/v4/me", token=token, expected=(401,))
        provider = self.provider
        _, headers, _ = http("GET", self.a.base + "/api/v4/auth/oauth2/scenario/redirect", expected=(302,))
        query = urllib.parse.parse_qs(urllib.parse.urlsplit(headers["Location"]).query)
        provider.challenge = query["code_challenge"][0]
        callback = "/api/v4/auth/oauth2/scenario/callback?" + urllib.parse.urlencode(
            {"state": query["state"][0], "code": provider.code})
        _, headers, _ = http("GET", self.b.base + callback, expected=(302,))
        one_time = urllib.parse.parse_qs(urllib.parse.urlsplit(headers["Location"]).query)["token"][0]
        http("GET", self.a.base + callback, expected=(400,))
        _, _, body = http("POST", self.a.base + "/api/v4/auth/oauth2/token", {"token": one_time}, expected=(201,))
        oauth_jwt = json.loads(body)["data"]["token"]
        http("GET", self.b.base + "/api/v4/me", token=oauth_jwt)
        http("POST", self.b.base + "/api/v4/auth/oauth2/token", {"token": one_time}, expected=(401,))
        self.record("cross-replica login, export, logout, OAuth PKCE and replay rejection")
        return self.login(self.b)

    def export_check(self, token):
        path = "/api/v4/exports/" + self.export_id
        http("GET", self.b.base + path, token=token)
        _, _, payload = http("GET", self.b.base + path + "/download", token=token)
        if self.run_id.encode() not in payload:
            raise ScenarioError("cross-replica export content mismatch")

    def marker(self):
        if self.sql("SELECT to_regclass('public.homer_maintenance_schedule')")[0][0] is None:
            return None
        rows = self.sql("SELECT completed_at FROM public.homer_maintenance_schedule WHERE task=%s", (SCHEDULE_KEY,))
        return rows[0][0] if rows else None

    def file_count(self):
        return self.sql("SELECT count(*) FROM public.ducklake_data_file WHERE end_snapshot IS NULL")[0][0]

    @contextlib.contextmanager
    def file_gate(self):
        with self.pg.connect(self.dsns[1]) as connection:
            connection.execute("SET LOCAL lock_timeout = '10s'")
            connection.execute("LOCK TABLE public.ducklake_data_file IN SHARE MODE")
            yield connection.info.backend_pid
            # Context exits commit/release after the owning process is killed.

    def blocked(self, gate_pid):
        rows = self.sql("SELECT pid FROM pg_stat_activity WHERE datname=current_database() "
                        "AND %s = ANY(pg_blocking_pids(pid))", (gate_pid,))
        return [row[0] for row in rows]

    def wait_sessions_closed(self, pids):
        self.wait("killed writer's blocked sessions closed",
                  lambda: not self.sql("SELECT pid FROM pg_stat_activity WHERE pid=ANY(%s)", (pids,)))

    def survivor_compacts(self, previous):
        def completed():
            marker = self.marker()
            return marker if marker and (previous is None or marker > previous) else None
        marker = self.wait("surviving replica completes compaction", completed)
        self.counts()
        return marker

    def flush_crash(self):
        previous, before = self.marker(), self.objects()
        with self.file_gate() as gate:
            pending = self.ingest(self.a, "interrupted-flush", self.args.rows)
            pids = self.wait("writer flush blocked at catalog commit", lambda: self.blocked(gate))
            uploaded = self.wait("real Parquet uploaded before blocked flush commit", lambda: self.objects() - before)
            self.a.stop(crash=True)
        self.wait_sessions_closed(pids)
        counts = self.counts()
        self.uncertain = pending - counts.keys()
        self.release_maintenance()
        marker = self.survivor_compacts(previous)
        self.record("writer killed during flush; survivor compacts",
                    blocked_backend_pids=pids, uploaded_objects=sorted(uploaded),
                    accepted_not_committed=len(self.uncertain), survivor_pid=self.b.process.pid,
                    completion_marker=marker)

    def compaction_crash(self):
        self.hold_maintenance()
        self.a.start()
        self.seed("before-compaction-crash")
        if self.file_count() < 2:
            raise ScenarioError("need multiple real files to exercise compaction")
        previous, before = self.marker(), self.objects()
        log_offset = self.a.log_path.stat().st_size
        # Freeze B only to select A as the compactor. B keeps the same PID and
        # is resumed in finally; it is never killed or restarted for failover.
        self.b.pause()
        try:
            with self.file_gate() as gate:
                self.release_maintenance()
                self.wait("A starts actual merge work",
                          lambda: "Merge adjacent files" in self.a.log_path.read_bytes()[log_offset:].decode(errors="replace"))
                pids = self.wait("compaction blocked at catalog commit", lambda: self.blocked(gate))
                uploaded = self.wait("merged Parquet uploaded before catalog commit", lambda: self.objects() - before)
                self.a.stop(crash=True)
            self.wait_sessions_closed(pids)
        finally:
            self.b.resume()
        marker = self.survivor_compacts(previous)
        self.record("writer killed during compaction; survivor compacts",
                    blocked_backend_pids=pids, uploaded_objects=sorted(uploaded),
                    survivor_pid=self.b.process.pid, completion_marker=marker)

    def schedule_failure(self):
        self.release_maintenance()
        self.survivor_compacts(None)
        self.hold_maintenance()
        self.seed("schedule-failure")
        before = self.file_count()
        if before < 2:
            raise ScenarioError("schedule failure requires multiple files")
        self.sql("ALTER TABLE public.homer_maintenance_schedule RENAME COLUMN completed_at TO scenario_broken")
        try:
            self.release_maintenance()
            self.wait("real merge despite unreadable schedule", lambda: self.file_count() < before)
            self.counts()
        finally:
            self.hold_maintenance()
            self.sql("ALTER TABLE public.homer_maintenance_schedule RENAME COLUMN scenario_broken TO completed_at")
        self.record("schedule-read failure permits real compaction", files_before=before,
                    files_after=self.file_count())

    def contention_and_buffers(self, replica, other):
        mode = "central" if replica.name == "b" else "per-table"
        self.hold_maintenance()
        self.seed("contention-" + mode, replica)
        previous, before = self.marker(), self.objects()
        offset = replica.log_path.stat().st_size
        pending = set()
        other.pause()
        try:
            with self.file_gate() as gate:
                self.release_maintenance()
                self.wait("actual merge for " + mode,
                          lambda: "Merge adjacent files" in replica.log_path.read_bytes()[offset:].decode(errors="replace"))
                pids = self.wait("merge catalog barrier", lambda: self.blocked(gate))
                self.wait("merge output uploaded", lambda: self.objects() - before)
                http("POST", f"http://127.0.0.1:{replica.flight + 1}/vacuum",
                     token=self.node_token, expected=(409,))
                env = self.child_env()
                env.update({"SCENARIO_DATA_PATH": self.data_path,
                            "SCENARIO_SPILL_PATH": str(self.output / "tier-spill"),
                            "AWS_ENDPOINT_URL_S3": self.args.endpoint, "AWS_REGION": self.args.region})
                # Capture potentially sensitive library logs in the private evidence directory.
                log = self.output / ("tier-" + mode + ".log")
                with log.open("wb") as output:
                    log.chmod(0o600)
                    result = subprocess.run([str(self.args.maintenance_probe)], env=env,
                                            cwd=self.output, stdout=output, stderr=subprocess.STDOUT,
                                            timeout=self.args.timeout)
                if result.returncode or b"SCENARIO_TIER_CONTENTION_PASSED" not in log.read_bytes():
                    raise ScenarioError("tier contention probe failed; see " + log.name)
                # More than two buffer slots and multiple timer ticks while this
                # writer's catalog mutex is occupied by the real merge.
                for cycle in range(5):
                    pending.update(self.ingest(replica, f"buffer-{mode}-{cycle}", self.args.rows))
                    time.sleep(1.1)
                def buffered():
                    query = ("SELECT cid, count(*) AS n FROM ("
                             "SELECT cid FROM mem_hep_proto_1_call_a UNION ALL "
                             "SELECT cid FROM mem_hep_proto_1_call_b) buffers GROUP BY cid")
                    _, _, body = http("POST", f"http://127.0.0.1:{replica.flight + 1}/query",
                                      {"sql": query}, self.node_token)
                    result = json.loads(body)
                    if not result.get("success"):
                        raise ScenarioError("buffer observation query failed")
                    rows = {row["cid"]: int(row["n"]) for row in result.get("data", [])}
                    check_rows(rows, set(), pending)
                    return pending <= rows.keys()
                # Poll partial processing without weakening the exact-once check.
                self.wait("all stress rows processed while merge holds catalog lock", buffered)
                if not self.blocked(gate):
                    raise ScenarioError("merge barrier disappeared during buffer stress")
        finally:
            other.resume()
        self.wait("all buffer stress rows durable", lambda: pending <= self.counts().keys())
        self.survivor_compacts(previous)
        self.hold_maintenance()
        self.record("vacuum/tiering contention and buffer stress: " + mode,
                    flush_queue=mode == "central", rows=len(pending), timer_cycles=5,
                    blocked_backend_pids=pids, writer_pid=replica.process.pid)

    def run(self):
        if not self.args.no_faults:
            probe = self.args.maintenance_probe or self.args.binary.with_name("stateless-maintenance-probe")
            self.args.maintenance_probe = probe.expanduser().resolve()
            if not probe.is_file() or not os.access(probe, os.X_OK):
                raise ScenarioError("build stateless-maintenance-probe or supply --maintenance-probe; see README")
        self.provision()
        self.hold_maintenance()
        self.provider = OAuthProvider()
        self.a, self.b = Replica(self, "a"), Replica(self, "b")
        self.a.start()
        self.b.start()
        self.seed("baseline")
        token = self.auth_and_exports()
        self.record("ingest on A, search through B's node and coordinator", rows=self.search(token))
        if self.args.no_faults:
            self.release_maintenance()
            self.survivor_compacts(None)
            self.report["status"] = "passed_smoke_only"
            self.report["fault_tests"] = "explicitly skipped with --no-faults"
        else:
            original_b = self.b.process.pid
            self.schedule_failure()
            self.contention_and_buffers(self.a, self.b)
            self.contention_and_buffers(self.b, self.a)
            self.flush_crash()
            self.export_check(token)
            self.compaction_crash()
            if self.b.process.pid != original_b:
                raise ScenarioError("surviving replica was replaced")
            self.export_check(token)
            self.search(token)
            # Restart the killed replica and verify the original shared JWT.
            self.hold_maintenance()
            self.a.start()
            http("GET", self.a.base + "/api/v4/me", token=token)
            self.record("shared JWT survives both writer crashes and restart")
            self.report["status"] = "passed"
        self.counts()
        self.save()

    def close(self):
        errors = []
        actions = [replica.stop for replica in (self.a, self.b) if replica]
        actions.append(self.release_maintenance)
        if self.provider:
            actions.append(self.provider.close)
        if self.args.cleanup:
            actions.append(self.cleanup)
        for action in actions:
            try:
                action()
            except Exception as exc:
                errors.append(str(exc))
        if errors:
            raise ScenarioError("; ".join(errors))

    def cleanup(self):
        from psycopg import sql
        errors = []
        # Attempt database cleanup even if cloud listing/deletion fails.
        try:
            pages = self.s3.get_paginator("list_objects_v2").paginate(Bucket=self.args.bucket, Prefix=self.prefix)
            for page in pages:
                for item in page.get("Contents", []):
                    if not item["Key"].startswith(self.prefix):
                        raise ScenarioError("object listing escaped this run's prefix")
                    try:
                        self.s3.delete_object(Bucket=self.args.bucket, Key=item["Key"])
                    except Exception as exc:
                        errors.append(str(exc))
        except Exception as exc:
            errors.append(str(exc))
        try:
            with self.pg.connect(self.admin_dsn, autocommit=True) as connection:
                for name in self.created:
                    try:
                        connection.execute(sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name)))
                    except Exception as exc:
                        errors.append(str(exc))
        except Exception as exc:
            errors.append(str(exc))
        self.report["cleanup"] = ("failed" if errors else
                                  "deleted current objects under run prefix and created databases")
        self.save()
        if errors:
            raise ScenarioError("; ".join(errors))

    def safe_error(self, error):
        message = str(error)
        for value in [os.environ["HOMER_SCENARIO_POSTGRES_DSN"], self.admin_dsn, *self.dsns, self.password, self.jwt_secret, self.node_token,
                      os.environ["AWS_ACCESS_KEY_ID"],
                      os.environ["AWS_SECRET_ACCESS_KEY"]]:
            if value:
                message = message.replace(value, "[redacted]")
        # psycopg can format a DSN password independently of the original DSN.
        password = self.pg.conninfo.conninfo_to_dict(self.admin_dsn).get("password", "")
        if password:
            message = message.replace(password, "[redacted]")
        return message


DEFAULT_ENV_PATH = Path(__file__).resolve().with_name(".env")


def default_env_file():
    if DEFAULT_ENV_PATH.is_file():
        return DEFAULT_ENV_PATH
    legacy = DEFAULT_ENV_PATH.parent.parent / ".env"
    return legacy if legacy.is_file() else DEFAULT_ENV_PATH


def arguments(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="built homer-core executable")
    parser.add_argument("--maintenance-probe", type=Path, help="tier probe executable (default: beside --binary)")
    bucket = os.environ.get("AWS_S3_BUCKET")
    parser.add_argument("--bucket", default=bucket, required=not bucket,
                        help="existing S3 bucket (default: AWS_S3_BUCKET); never created or deleted")
    endpoint = os.environ.get("AWS_ENDPOINT_URL_S3")
    region = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
    parser.add_argument("--env-file", type=Path, default=default_env_file(),
                        help="dotenv file (default: beside this script; legacy scripts/.env also supported)")
    parser.add_argument("--endpoint", default=endpoint, required=not endpoint,
                        help="S3-compatible origin (default: AWS_ENDPOINT_URL_S3)")
    parser.add_argument("--region", default=region,
                        help="signing region (default: AWS_REGION, AWS_DEFAULT_REGION, or us-east-1)")
    parser.add_argument("--prefix", default="homer-stateless-scenarios", help="parent prefix; a unique run ID is always appended")
    parser.add_argument("--output", type=Path, default=Path("local/stateless-scenarios"))
    parser.add_argument("--rows", type=int, default=64, help="rows per phase, multiple of 8, between 16 and 4096")
    parser.add_argument("--timeout", type=float, default=120, help="seconds per readiness, fault, or verification operation")
    parser.add_argument("--memory-limit", default="256MB", help="DuckDB limit per instance")
    parser.add_argument("--no-faults", action="store_true", help="smoke test only; report does not claim crash coverage")
    parser.add_argument("--cleanup", action="store_true", help="delete this run's databases and current objects after stopping processes")
    args = parser.parse_args(argv)
    args.binary = args.binary.expanduser().resolve()
    if not args.binary.is_file() or not os.access(args.binary, os.X_OK):
        parser.error("--binary must be an executable file")
    if not 16 <= args.rows <= 4096 or args.rows % 8:
        parser.error("--rows must be a multiple of eight between 16 and 4096")
    if args.timeout <= 0:
        parser.error("--timeout must be positive")
    endpoint = urllib.parse.urlsplit(args.endpoint)
    if endpoint.scheme not in ("http", "https") or not endpoint.netloc or endpoint.path not in ("", "/") or endpoint.username or endpoint.query or endpoint.fragment:
        parser.error("--endpoint must be an HTTP(S) origin without credentials, path or query")
    if "/" in args.bucket or not args.bucket:
        parser.error("--bucket must be a bucket name, not a URL/path")
    if not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9/_-]*", args.prefix.strip("/")):
        parser.error("--prefix must use letters, digits, underscores, hyphens and slashes")
    args.endpoint = args.endpoint.rstrip("/")
    return args


def load_env_file(path):
    if not path.is_file():
        return False
    try:
        from dotenv import load_dotenv
    except ImportError:
        raise SystemExit("Install scripts/stateless-scenario/requirements.txt to load the dotenv file.")
    # Keep literal secret characters; never execute or source shell contents.
    return load_dotenv(path, override=False, interpolate=False)


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    if "--help" in argv or "-h" in argv:
        arguments(argv)
    bootstrap = argparse.ArgumentParser(add_help=False)
    bootstrap.add_argument("--env-file", type=Path, default=default_env_file())
    env_args, _ = bootstrap.parse_known_args(argv)
    load_env_file(env_args.env_file)
    args = arguments(argv)
    if os.name != "posix" or not hasattr(signal, "SIGSTOP"):
        raise SystemExit("This harness requires POSIX process signals (run it on Linux).")
    required = ("HOMER_SCENARIO_POSTGRES_DSN", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY")
    missing = [key for key in required if not os.environ.get(key)]
    if missing:
        raise SystemExit("Set environment variables: " + ", ".join(missing))
    try:
        runner = Runner(args)
    except ImportError:
        raise SystemExit("Install scripts/stateless-scenario/requirements.txt in a Python virtual environment.")
    except Exception as exc:
        raise SystemExit(f"Initialization failed ({type(exc).__name__}); check the DSN, endpoint and output path.")
    try:
        print(f"Run {runner.run_id}; evidence: {runner.output}", flush=True)
        runner.run()
    except (Exception, KeyboardInterrupt) as exc:
        runner.report["status"] = "failed"
        runner.report["error"] = ("interrupted by operator" if isinstance(exc, KeyboardInterrupt)
                                  else runner.safe_error(exc))
        runner.save()
        print("FAIL " + runner.report["error"], file=sys.stderr, flush=True)
    finally:
        try:
            runner.close()
        except Exception as exc:
            runner.report["status"] = "failed"
            runner.report["cleanup_error"] = runner.safe_error(exc)
            runner.save()
            print("FAIL cleanup: " + runner.report["cleanup_error"], file=sys.stderr)
    print(f"Result: {runner.report['status']}; {runner.output / 'report.json'}", flush=True)
    return 0 if runner.report["status"].startswith("passed") else 1


if __name__ == "__main__":
    sys.exit(main())
