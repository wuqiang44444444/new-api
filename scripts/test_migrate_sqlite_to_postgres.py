"""Deterministic migration regression tests; integration uses a disposable PG server."""

from contextlib import closing, redirect_stdout, redirect_stderr
import importlib.util
import io
import os
from pathlib import Path
import sqlite3
import sys
import tempfile
import unittest
from unittest.mock import patch


spec = importlib.util.spec_from_file_location("sqlite_postgres_migration", Path(__file__).with_name("migrate-sqlite-to-postgres.py"))
migration = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = migration
spec.loader.exec_module(migration)


def fixture(path):
    with closing(sqlite3.connect(path)) as db, db:
        db.executescript("""
            CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, quota INTEGER NOT NULL,
                used_quota INTEGER, contract_mode numeric, username text, deleted_at datetime);
            CREATE TABLE channels (id INTEGER PRIMARY KEY, "group" varchar(64), channel_info json);
            CREATE TABLE ledger (id INTEGER PRIMARY KEY AUTOINCREMENT, money decimal(10,6),
                real_value real, empty text, missing text, raw blob, secret text);
            CREATE TABLE abilities ("group" text, model text, channel_id integer,
                enabled numeric, PRIMARY KEY ("group",model,channel_id));
            CREATE UNIQUE INDEX users_active_name ON users(username) WHERE deleted_at IS NULL;
            CREATE INDEX ledger_order ON ledger(money DESC,id ASC);
        """)
        db.execute("INSERT INTO users VALUES (?,?,?,?,?,?)", (1, 5_000_000_001, 9_000_000_003, 0, "客户", None))
        db.execute("INSERT INTO users VALUES (?,?,?,?,?,?)", (50, 0, 0, 1, "removed", None))
        db.execute("DELETE FROM users WHERE id=50")  # Retain the AUTOINCREMENT high-water mark.
        db.execute("INSERT INTO channels VALUES (?,?,?)", (9, ",".join(["group"] * 20), b'{ "multi": false, "x": [0,1] }'))
        db.execute("INSERT INTO ledger VALUES (?,?,?,?,?,?,?)", (3, 0.123456, 1.23456789012345, "", None, b"\x00\xff", "private-test-value\n\\N\t中文"))
        db.execute("INSERT INTO abilities VALUES (?,?,?,?)", ("default", "model", 9, 1))


class SourceTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / "source.db"
        fixture(self.path)

    def test_snapshot_includes_committed_wal_and_never_overwrites(self):
        with closing(sqlite3.connect(self.path)) as source, source:
            source.execute("PRAGMA journal_mode=WAL")
            source.execute("INSERT INTO users VALUES (?,?,?,?,?,?)", (51, 7, 8, 1, "wal-user", None))
            source.commit()
            with self.assertRaises(migration.MigrationError):
                migration.open_source(self.path)
            output = Path(self.tmp.name) / "backup.db"
            with redirect_stdout(io.StringIO()):
                migration.snapshot(self.path, output)
            with closing(sqlite3.connect(output)) as copied:
                self.assertEqual(copied.execute("SELECT quota FROM users WHERE id=51").fetchone(), (7,))
            with self.assertRaises(FileExistsError):
                migration.snapshot(self.path, output)
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)

    def test_rejects_bad_boolean_without_printing_private_value(self):
        with closing(sqlite3.connect(self.path)) as source, source:
            source.execute("UPDATE users SET contract_mode='private-test-value'")
        output = io.StringIO()
        with redirect_stdout(output), redirect_stderr(output):
            code = migration.main(["preflight", "--source", str(self.path), "--report", str(Path(self.tmp.name) / "report.json")])
        self.assertEqual(code, 1)
        self.assertNotIn("private-test-value", output.getvalue())
        self.assertIn("users.contract_mode", output.getvalue())

    def test_rejects_unregistered_numeric_and_unsupported_schema(self):
        with self.assertRaises(migration.MigrationError):
            migration.postgres_type("new_table", "flag", "numeric")
        with closing(sqlite3.connect(self.path)) as source, source:
            source.execute("CREATE TRIGGER user_trigger AFTER INSERT ON users BEGIN SELECT 1; END")
        with closing(migration.open_source(self.path)) as source:
            with self.assertRaises(migration.MigrationError):
                migration.read_schema(source)

    def test_rejects_rounding_truncation_and_timestamp_precision_loss(self):
        for value, kind in [(0.1234567, "numeric(10,6)"), ("too-long", "varchar(3)"),
                            ("2026-10-14T10:20:30.123456789Z", "timestamp with time zone"),
                            ("nul\x00text", "text")]:
            with self.subTest(kind=kind), self.assertRaises(migration.MigrationError):
                migration.convert(value, migration.Column("x", kind, False, None, 0))


@unittest.skipUnless(os.environ.get("ONEAPI_MIGRATION_TEST_DSN"), "Requires a disposable PostgreSQL server")
class PostgreSQLTests(unittest.TestCase):
    def setUp(self):
        import psycopg
        from psycopg.conninfo import make_conninfo
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / "source.db"
        fixture(self.path)
        self.source = migration.open_source(self.path)
        self.addCleanup(self.source.close)
        self.source.execute("BEGIN")
        self.tables = migration.read_schema(self.source)
        self.fingerprint = migration.file_hash(self.path)
        with redirect_stdout(io.StringIO()):
            self.report = migration.inspect(self.source, self.tables, self.fingerprint)
        dsn = os.environ["ONEAPI_MIGRATION_TEST_DSN"]
        self.admin = psycopg.connect(dsn, autocommit=True)
        self.addCleanup(self.admin.close)
        self.name = "oneapi_migration_" + str(os.getpid()) + "_" + self._testMethodName.removeprefix("test_")[:30]
        self.admin.execute("CREATE DATABASE " + migration.quote(self.name))
        self.addCleanup(self.drop_database)
        self.pg = psycopg.connect(make_conninfo(dsn, dbname=self.name), autocommit=True)
        self.addCleanup(self.pg.close)

    def drop_database(self):
        self.admin.execute("DROP DATABASE " + migration.quote(self.name))

    def run_import(self):
        with redirect_stdout(io.StringIO()):
            migration.import_database(self.source, self.tables, self.report, self.pg, self.path, self.fingerprint)

    def test_round_trip_funds_json_nulls_ids_and_sequences(self):
        self.run_import()
        self.assertEqual(self.pg.execute("SELECT quota,used_quota,contract_mode FROM users").fetchone(), (5_000_000_001, 9_000_000_003, False))
        self.assertEqual(self.pg.execute("SELECT length(\"group\"),channel_info::text FROM channels").fetchone(), (119, '{ "multi": false, "x": [0,1] }'))
        self.assertEqual(self.pg.execute("SELECT empty,missing,raw FROM ledger").fetchone(), ("", None, b"\x00\xff"))
        self.assertEqual(self.pg.execute("INSERT INTO users(quota,username) VALUES (1,'new') RETURNING id").fetchone(), (51,))
        self.assertEqual(self.pg.execute("INSERT INTO ledger(money) VALUES (0) RETURNING id").fetchone(), (4,))
        with self.assertRaises(Exception):
            self.pg.execute("INSERT INTO users(quota,username) VALUES (1,'客户')")

    def test_nonempty_target_is_not_modified(self):
        self.pg.execute("CREATE TABLE keep (id bigint PRIMARY KEY)")
        self.pg.execute("INSERT INTO keep VALUES (99)")
        with self.assertRaises(migration.MigrationError):
            self.run_import()
        self.assertEqual(self.pg.execute("SELECT id FROM keep").fetchall(), [(99,)])
        self.assertEqual(self.pg.execute("SELECT to_regclass('public.users')").fetchone(), (None,))

    def test_verification_failure_rolls_back_all_ddl_and_copy(self):
        with patch.object(migration, "verify_tables", side_effect=migration.MigrationError("injected verification failure")):
            with self.assertRaises(migration.MigrationError):
                self.run_import()
        self.assertEqual(self.pg.execute("SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'").fetchone(), (0,))

    def test_read_only_verification_detects_modified_funds(self):
        self.run_import()
        self.pg.execute("UPDATE users SET quota=quota+1")
        with self.pg.transaction(), redirect_stdout(io.StringIO()):
            self.pg.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
            with self.assertRaises(migration.MigrationError):
                migration.verify_tables(self.pg, self.tables, self.report["tables"])


if __name__ == "__main__":
    unittest.main()
