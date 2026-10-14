#!/usr/bin/env python3
"""Offline, all-table SQLite -> PostgreSQL migration. Never starts the application.

snapshot/preflight use the standard library. import/verify require psycopg 3.
Only an empty, dedicated PostgreSQL database may be imported. DDL, COPY, indexes,
sequence resets and full data verification share one transaction.
"""

import argparse
from contextlib import closing
from dataclasses import dataclass
from datetime import datetime, timezone
from decimal import Decimal, InvalidOperation
import hashlib
import json
import math
import os
from pathlib import Path
import re
import sqlite3
import sys


# These are schema contracts, not guesses based on the values in a column.
BOOLEAN_COLUMNS = frozenset("""
abilities.enabled audit_logs.success authz_roles.built_in authz_roles.enabled
billing_statement_artifacts.staged billing_statement_maintenance.enabled
custom_oauth_providers.enabled customer_contract_templates.enabled
customer_contracts.enabled customer_export_jobs.cancel_requested
customer_export_jobs.cancel_wait error_report_schedules.enabled logs.is_stream
passkey_credentials.clone_warning passkey_credentials.user_present
passkey_credentials.user_verified passkey_credentials.backup_eligible
passkey_credentials.backup_state subscription_plans.enabled
subscription_plans.allow_balance_pay subscription_plans.allow_wallet_overflow
task_billing_deliveries.usage_reported task_billing_deliveries.suppress_log
task_create_attempts.token_quota_tracked task_create_attempts.token_quota_held
task_plugins.enabled task_plugins.active task_request_evidence_events.complete
task_request_evidence_events.redacted task_request_evidences.body_expired
tasks.video_funding_ready tokens.unlimited_quota tokens.model_limits_enabled
tokens.cross_group_retry two_fa_backup_codes.is_used two_fas.is_enabled
user_subscriptions.allow_wallet_overflow users.contract_mode
""".split())


class MigrationError(Exception):
    """An error safe to display without database values or connection details."""


@dataclass
class Column:
    name: str
    pg_type: str
    not_null: bool
    default: object
    primary_order: int
    source_type: str = ""
    allow_timestamp_truncation: bool = False


@dataclass
class Index:
    name: str
    unique: bool
    columns: list
    predicate: str = ""


@dataclass
class Table:
    name: str
    columns: list
    indexes: list
    sequence_floor: int = 0

    @property
    def primary(self):
        return sorted((c for c in self.columns if c.primary_order),
                      key=lambda c: c.primary_order)

    @property
    def auto_id(self):
        return len(self.primary) == 1 and self.primary[0].name == "id" and self.primary[0].pg_type == "bigint"


def quote(name):
    if not name or "\x00" in name or len(name.encode("utf-8")) > 63:
        raise MigrationError("Unsupported identifier: empty, NUL, or more than 63 UTF-8 bytes.")
    return '"' + name.replace('"', '""') + '"'


def source_path(value):
    path = Path(value).expanduser().resolve(strict=True)
    if not path.is_file():
        raise MigrationError("Source must be an existing SQLite file.")
    return path


def file_hash(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def open_source(path, require_snapshot=True):
    wal = Path(str(path) + "-wal")
    if require_snapshot and wal.exists() and wal.stat().st_size:
        raise MigrationError("Source has committed/pending WAL bytes; first use snapshot to make a standalone backup.")
    db = sqlite3.connect(path.as_uri() + "?mode=ro", uri=True, timeout=10)
    db.execute("PRAGMA query_only=ON")
    return db


def private_output(path):
    return os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "w", encoding="utf-8")


def snapshot(path, output):
    # Exclusive creation: a typo must not destroy an existing backup.
    fd = os.open(output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)
    with closing(open_source(path, require_snapshot=False)) as source, closing(sqlite3.connect(output)) as target:
        source.backup(target, pages=256)
        if target.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
            raise MigrationError("Snapshot integrity check failed; do not use the output.")
        target.execute("PRAGMA journal_mode=DELETE")
    print("snapshot=complete sha256=" + file_hash(Path(output)), flush=True)


def postgres_type(table, name, declared):
    kind = declared.strip().lower()
    # The application validates the complete CSV list at 128 characters.
    # Widen the historical SQLite declaration; never truncate existing values.
    if table == "channels" and name == "group" and kind in ("varchar(64)", "varchar(128)", "text"):
        return "varchar(128)"
    if table + "." + name in BOOLEAN_COLUMNS:
        if kind not in ("numeric", "boolean", "bool", "integer"):
            raise MigrationError(f"Boolean schema drift at {table}.{name}.")
        return "boolean"
    if kind in ("integer", "int", "bigint", "smallint", "tinyint"):
        return "bigint"  # SQLite INTEGER has 64-bit capacity, including wallets.
    if kind in ("real", "double", "double precision", "float"):
        return "double precision"  # PostgreSQL real is only float32.
    if kind in ("text", "clob"):
        return "text"
    if kind in ("json", "blob"):
        return {"json": "json", "blob": "bytea"}[kind]
    if kind in ("datetime", "timestamp", "timestamptz"):
        return "timestamp with time zone"
    if re.fullmatch(r"(?:var)?char\([1-9][0-9]*\)", kind):
        return kind
    if re.fullmatch(r"decimal\([1-9][0-9]*,[0-9]+\)", kind):
        precision, scale = map(int, re.findall(r"[0-9]+", kind))
        if scale <= precision <= 1000:
            return kind.replace("decimal", "numeric")
    # Bare numeric in GORM SQLite schemas is commonly boolean. Never guess.
    raise MigrationError(f"Unregistered SQLite type at {table}.{name}; review and extend the explicit schema mapping.")


def default_sql(column):
    value = column.default
    if value is None:
        return ""
    text = value.strip()
    while text.startswith("(") and text.endswith(")"):
        text = text[1:-1].strip()
    if text.upper() == "NULL":
        return " DEFAULT NULL"
    if column.pg_type == "boolean":
        if text.lower() in ("0", "false", "1", "true"):
            return " DEFAULT " + ("true" if text.lower() in ("1", "true") else "false")
        raise MigrationError("Unsupported boolean default.")
    if re.fullmatch(r"[+-]?[0-9]+(?:\.[0-9]+)?", text):
        return " DEFAULT " + text
    if len(text) >= 2 and text[0] in "\"'" and text[-1] == text[0]:
        literal = text[1:-1].replace(text[0] * 2, text[0])
        if "\x00" not in literal:
            return " DEFAULT '" + literal.replace("'", "''") + "'"
    raise MigrationError("Unsupported SQLite default expression; no default is silently dropped.")


def read_schema(db, allow_timestamp_truncation=False):
    if db.execute("PRAGMA integrity_check").fetchall() != [("ok",)]:
        raise MigrationError("SQLite integrity_check failed.")
    if db.execute("PRAGMA foreign_key_check").fetchone():
        raise MigrationError("SQLite foreign_key_check failed.")
    if db.execute("SELECT 1 FROM sqlite_master WHERE type IN ('view','trigger') LIMIT 1").fetchone():
        raise MigrationError("Views/triggers require a reviewed conversion; automatic import refused.")
    sequences = {}
    if db.execute("SELECT 1 FROM sqlite_master WHERE name='sqlite_sequence'").fetchone():
        sequences = dict(db.execute("SELECT name,seq FROM sqlite_sequence"))
    tables = []
    names = set()
    for table_name, ddl in db.execute("SELECT name,sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name").fetchall():
        quote(table_name)
        if re.search(r"\b(CHECK|REFERENCES|COLLATE|WITHOUT\s+ROWID|STRICT|VIRTUAL)\b", ddl, re.I):
            raise MigrationError(f"Unsupported table feature at {table_name}; explicit conversion required.")
        if db.execute(f"PRAGMA foreign_key_list({quote(table_name)})").fetchone():
            raise MigrationError(f"Foreign keys require explicit conversion at {table_name}.")
        columns = []
        for _, name, declared, required, default, primary, hidden in db.execute(f"PRAGMA table_xinfo({quote(table_name)})"):
            quote(name)
            if hidden:
                raise MigrationError(f"Generated/hidden column at {table_name}.{name}.")
            col = Column(name, postgres_type(table_name, name, declared), bool(required or primary), default, primary,
                         declared, allow_timestamp_truncation)
            default_sql(col)
            columns.append(col)
        table = Table(table_name, columns, [], int(sequences.get(table_name, 0)))
        if not table.primary:
            raise MigrationError(f"Table {table_name} has no primary key; deterministic verification requires one.")
        for _, name, unique, origin, partial in db.execute(f"PRAGMA index_list({quote(table_name)})").fetchall():
            if origin == "pk":
                continue
            details = [r for r in db.execute(f"PRAGMA index_xinfo({quote(name)})") if r[5]]
            if any(r[1] < 0 or r[4] != "BINARY" for r in details):
                raise MigrationError(f"Expression/custom collation index at {table_name}; explicit conversion required.")
            predicate = ""
            if partial:
                sql = db.execute("SELECT sql FROM sqlite_master WHERE type='index' AND name=?", (name,)).fetchone()[0]
                # The existing soft-delete uniqueness contract is the only supported predicate.
                match = re.search(r"\bWHERE\s+[`\"]?([A-Za-z_][A-Za-z_0-9]*)[`\"]?\s+IS\s+NULL\s*$", sql, re.I)
                if not match or match[1] not in {c.name for c in columns}:
                    raise MigrationError(f"Unsupported partial index at {table_name}.")
                predicate = " WHERE " + quote(match[1]) + " IS NULL"
            if name.startswith("sqlite_autoindex_"):
                name = "migration_uq_" + hashlib.sha256(name.encode()).hexdigest()[:24]
            quote(name)
            if name in names:
                raise MigrationError("Index identifiers collide across tables.")
            names.add(name)
            table.indexes.append(Index(name, bool(unique), [(r[2], bool(r[3])) for r in details], predicate))
        tables.append(table)
    if not tables:
        raise MigrationError("Source contains no user tables.")
    # PostgreSQL tables, indexes and sequences share a namespace.
    objects = {t.name for t in tables}
    objects.update(t.name + "_pkey" for t in tables)
    objects.update(t.name + "_id_seq" for t in tables if t.auto_id)
    for name in objects:
        quote(name)
    if len(objects) != len(tables) * 2 + sum(t.auto_id for t in tables) or names & objects:
        raise MigrationError("Table/primary-key/index/sequence names collide.")
    return tables


def convert(value, column):
    if value is None:
        if column.not_null:
            raise MigrationError("NULL in a required/primary-key column.")
        return None
    kind = column.pg_type
    if kind == "boolean":
        if type(value) not in (int, bool) or value not in (0, 1):
            raise MigrationError("Boolean must be an integer 0 or 1.")
        return bool(value)
    if kind == "bigint":
        if type(value) is not int or not -(2**63) <= value < 2**63:
            raise MigrationError("Integer storage class/range mismatch.")
        return value
    if kind == "double precision":
        if type(value) not in (int, float) or not math.isfinite(value):
            raise MigrationError("Invalid/non-finite real value.")
        return float(value)
    if kind.startswith("numeric("):
        if type(value) not in (int, float) and not isinstance(value, Decimal):
            raise MigrationError("Invalid decimal storage class.")
        number = Decimal(str(value))
        precision, scale = map(int, re.findall(r"[0-9]+", kind))
        if not number.is_finite() or number != number.quantize(Decimal(1).scaleb(-scale)) or abs(number) >= Decimal(10) ** (precision - scale):
            raise MigrationError("Decimal would overflow or require rounding.")
        return number
    if kind == "bytea":
        if not isinstance(value, (bytes, memoryview)):
            raise MigrationError("Invalid binary storage class.")
        return bytes(value)
    if kind == "timestamp with time zone":
        if isinstance(value, datetime):
            date = value
        elif isinstance(value, str):
            # PostgreSQL stores microseconds. Reject loss of nonzero nanoseconds.
            fraction = re.search(r"\.([0-9]+)", value)
            if fraction and any(d != "0" for d in fraction[1][6:]) and not column.allow_timestamp_truncation:
                raise MigrationError("Timestamp would lose sub-microsecond precision.")
            try:
                date = datetime.fromisoformat(value.replace("Z", "+00:00"))
            except ValueError:
                raise MigrationError("Unsupported timestamp format.") from None
        else:
            raise MigrationError("Invalid timestamp storage class.")
        # GORM timestamps without an explicit offset use UTC in this migration.
        return date.replace(tzinfo=timezone.utc).astimezone(timezone.utc) if date.tzinfo is None else date.astimezone(timezone.utc)
    if kind == "json" and isinstance(value, (bytes, memoryview)):
        # GORM Valuer returns []byte, which SQLite stores as BLOB in JSON columns.
        value = bytes(value).decode("utf-8", errors="strict")
    if kind == "json" and type(value) in (int, float):
        value = json.dumps(value, allow_nan=False, separators=(",", ":"))
    if not isinstance(value, str) or "\x00" in value:
        raise MigrationError("Invalid text storage class or NUL byte.")
    value.encode("utf-8", errors="strict")
    if kind.startswith(("varchar(", "char(")):
        length = int(re.search(r"[0-9]+", kind)[0])
        if len(value) > length or kind.startswith("char(") and value.endswith(" "):
            raise MigrationError("Text length/trailing CHAR spaces would change during import.")
    if kind == "json":
        try:
            json.loads(value, parse_constant=invalid_json_constant)
        except (ValueError, RecursionError):
            raise MigrationError("Invalid JSON; no JSON value is replaced or repaired.") from None
    return value


def invalid_json_constant(_):
    raise ValueError("Nonstandard JSON numeric constant")


def normalized_row(row, table, position):
    converted = []
    for value, column in zip(row, table.columns):
        try:
            converted.append(convert(value, column))
        except (MigrationError, ValueError, OverflowError, InvalidOperation, UnicodeError) as error:
            # Do not include error text from JSON/decimal/date libraries or row values.
            message = str(error) if isinstance(error, MigrationError) else "Value cannot be represented losslessly."
            raise MigrationError(f"{table.name}.{column.name} row ordinal {position}: {message}") from None
    return tuple(converted)


def update_digest(digest, row):
    encoded = []
    for value in row:
        if value is None:
            encoded.append(["null"])
        elif isinstance(value, bool):
            encoded.append(["bool", value])
        elif isinstance(value, int):
            encoded.append(["int", str(value)])
        elif isinstance(value, float):
            encoded.append(["float", value.hex()])
        elif isinstance(value, Decimal):
            encoded.append(["decimal", format(value.normalize(), "f")])
        elif isinstance(value, datetime):
            encoded.append(["time", value.isoformat(timespec="microseconds")])
        elif isinstance(value, bytes):
            encoded.append(["bytes", value.hex()])
        else:
            encoded.append(["text", value])
    record = json.dumps(encoded, ensure_ascii=True, separators=(",", ":")).encode("ascii")
    digest.update(len(record).to_bytes(8, "big"))
    digest.update(record)


def select_sql(table, postgres=False):
    fields = []
    for column in table.columns:
        item = quote(column.name)
        if postgres and (column.pg_type == "json" or column.pg_type.startswith("char(")):
            item += "::text"
        fields.append(item)
    orders = []
    for col in table.primary:
        item = quote(col.name)
        if col.pg_type == "text" or col.pg_type.startswith(("varchar(", "char(")):
            item += ' COLLATE "C"' if postgres else ' COLLATE BINARY'
        orders.append(item)
    location = "public." if postgres else ""
    return f"SELECT {','.join(fields)} FROM {location}{quote(table.name)} ORDER BY {','.join(orders)}"


def table_digest(rows, table):
    digest = hashlib.sha256()
    count = 0
    for count, row in enumerate(rows, 1):
        update_digest(digest, normalized_row(row, table, count))
    return {"rows": count, "sha256": digest.hexdigest()}


def inspect(db, tables, fingerprint):
    report = {"format": "oneapi-sqlite-postgres-v1", "source_sha256": fingerprint,
              "table_count": len(tables), "tables": {}, "states": {},
              "schema": {}, "timestamp_truncations": {}, "application_blockers": []}
    for table in tables:
        report["schema"][table.name] = {
            "columns": [{"name": c.name, "sqlite_type": c.source_type, "postgres_type": c.pg_type,
                         "not_null": c.not_null, "primary_order": c.primary_order} for c in table.columns],
            "indexes": [{"name": i.name, "unique": i.unique, "columns": i.columns, "predicate": i.predicate} for i in table.indexes],
            "sequence_floor": table.sequence_floor,
        }
        for column in table.columns:
            if column.pg_type == "timestamp with time zone":
                truncated = 0
                for (value,) in db.execute(f"SELECT {quote(column.name)} FROM {quote(table.name)} WHERE {quote(column.name)} IS NOT NULL"):
                    fraction = re.search(r"\.([0-9]+)", value) if isinstance(value, str) else None
                    if fraction and any(d != "0" for d in fraction[1][6:]):
                        truncated += 1
                if truncated:
                    report["timestamp_truncations"][table.name + "." + column.name] = truncated
        report["tables"][table.name] = table_digest(db.execute(select_sql(table)), table)
        print(f"preflight table={table.name} rows={report['tables'][table.name]['rows']}", flush=True)
    # Counts only. Never export task bodies, credentials, object paths or option values.
    for name, columns in {"tasks": ["status", "billing_state"],
                          "task_create_attempts": ["status", "billing_hold_state"],
                          "system_tasks": ["status"]}.items():
        table = next((t for t in tables if t.name == name), None)
        if table:
            for column in columns:
                if column in {c.name for c in table.columns}:
                    report["states"][name + "." + column] = db.execute(f"SELECT {quote(column)},count(*) FROM {quote(name)} GROUP BY {quote(column)} ORDER BY {quote(column)}").fetchall()
    return report


def assert_unchanged_source(path, fingerprint):
    wal = Path(str(path) + "-wal")
    if file_hash(path) != fingerprint or wal.exists() and wal.stat().st_size:
        raise MigrationError("Source file/WAL changed during execution; use an isolated standalone snapshot.")


def connect_postgres(env, expected_database, writable):
    try:
        import psycopg
    except ImportError:
        raise MigrationError("Install psycopg 3 in an isolated virtualenv: pip install 'psycopg[binary]>=3.2,<4'.") from None
    dsn = os.environ.get(env)
    if not dsn:
        raise MigrationError(f"Set {env} via a private environment/service file; never put credentials in CLI arguments.")
    connection = psycopg.connect(dsn, connect_timeout=15, autocommit=True)
    with connection.cursor() as cursor:
        cursor.execute("SELECT current_database(),current_setting('server_encoding'),pg_is_in_recovery()")
        database, encoding, recovery = cursor.fetchone()
    if database != expected_database or encoding != "UTF8" or writable and recovery:
        connection.close()
        raise MigrationError("PostgreSQL database name, UTF8 encoding, or writable-primary check failed.")
    return connection


def assert_empty(cursor):
    cursor.execute("""SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
        WHERE n.nspname NOT IN ('pg_catalog','information_schema') AND n.nspname NOT LIKE 'pg_toast%'
        AND c.relkind IN ('r','p','v','m','S','f')""")
    if cursor.fetchone()[0]:
        raise MigrationError("Target is not empty; existing tables/views/sequences are never dropped or merged.")
    cursor.execute("SELECT count(*) FROM pg_extension WHERE extname <> 'plpgsql'")
    if cursor.fetchone()[0]:
        raise MigrationError("Use a dedicated empty database without application extensions.")


def create_table(cursor, table):
    definitions = []
    for column in table.columns:
        definition = quote(column.name) + " " + column.pg_type
        if column.not_null:
            definition += " NOT NULL"
        definition += default_sql(column)
        definitions.append(definition)
    definitions.append("CONSTRAINT " + quote(table.name + "_pkey") + " PRIMARY KEY (" + ",".join(quote(c.name) for c in table.primary) + ")")
    cursor.execute(f"CREATE TABLE public.{quote(table.name)} ({','.join(definitions)})")
    if table.auto_id:
        sequence = "public." + quote(table.name + "_id_seq")
        cursor.execute(f"CREATE SEQUENCE {sequence} OWNED BY public.{quote(table.name)}.id")
        sequence_literal = sequence.replace("'", "''")
        cursor.execute(f"ALTER TABLE public.{quote(table.name)} ALTER COLUMN id SET DEFAULT nextval('{sequence_literal}'::regclass)")


def create_indexes_and_sequence(cursor, table):
    for index in table.indexes:
        cols = ",".join(quote(name) + (" DESC" if desc else " ASC") for name, desc in index.columns)
        cursor.execute(f"CREATE {'UNIQUE ' if index.unique else ''}INDEX {quote(index.name)} ON public.{quote(table.name)} ({cols}){index.predicate}")
    if table.auto_id:
        cursor.execute(f"SELECT COALESCE(MAX(id),0) FROM public.{quote(table.name)}")
        maximum = max(cursor.fetchone()[0], table.sequence_floor)
        if maximum >= 2**63 - 1:
            raise MigrationError(f"Sequence exhausted at {table.name}.")
        cursor.execute("SELECT setval(%s::regclass,%s,%s)",
                       ('public.' + quote(table.name + "_id_seq"), max(1, maximum), maximum > 0))


def verify_tables(connection, tables, expected):
    for number, table in enumerate(tables):
        # Server-side cursors bound memory even for a large log table.
        with connection.cursor(name=f"migration_verify_{number}") as cursor:
            cursor.itersize = 2000
            cursor.execute(select_sql(table, postgres=True))
            actual = table_digest(cursor, table)
        if actual != expected[table.name]:
            raise MigrationError(f"Full row count/hash mismatch at {table.name}; no row values are displayed.")
        print(f"verify table={table.name} rows={actual['rows']} result=match", flush=True)


def import_database(db, tables, report, connection, path, fingerprint):
    with connection.transaction():
        with connection.cursor() as cursor:
            cursor.execute("SET LOCAL lock_timeout='10s'")
            cursor.execute("SET LOCAL search_path=public,pg_catalog")
            cursor.execute("SET LOCAL TimeZone='UTC'")
            cursor.execute("SET LOCAL standard_conforming_strings=on")
            # Cooperative lock prevents two copies of this tool from importing together.
            cursor.execute("SELECT pg_try_advisory_xact_lock(763147280051)")
            if not cursor.fetchone()[0]:
                raise MigrationError("Another migration is using this target database.")
            assert_empty(cursor)
            for table in tables:
                create_table(cursor, table)
                names = ",".join(quote(c.name) for c in table.columns)
                digest = hashlib.sha256()
                count = 0
                with cursor.copy(f"COPY public.{quote(table.name)} ({names}) FROM STDIN") as copy:
                    for count, row in enumerate(db.execute(select_sql(table)), 1):
                        converted = normalized_row(row, table, count)
                        update_digest(digest, converted)
                        copy.write_row(converted)
                if {"rows": count, "sha256": digest.hexdigest()} != report["tables"][table.name]:
                    raise MigrationError("Source changed between preflight and COPY.")
                create_indexes_and_sequence(cursor, table)
                print(f"import table={table.name} rows={count}", flush=True)
        verify_tables(connection, tables, report["tables"])
        assert_unchanged_source(path, fingerprint)
    # This message is emitted only after PostgreSQL acknowledges COMMIT.
    print("import=committed data_verification=all_tables_match application_acceptance=required", flush=True)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    for name in ("snapshot", "preflight", "import", "verify"):
        sub = commands.add_parser(name)
        sub.add_argument("--source", required=True)
        if name == "snapshot":
            sub.add_argument("--output", required=True)
        else:
            sub.add_argument("--report", required=True, help="New private JSON report file; never overwritten")
            sub.add_argument("--allow-timestamp-truncation", action="store_true",
                             help="Explicitly allow nanosecond timestamps to be truncated to PostgreSQL microseconds; count in report")
        if name in ("import", "verify"):
            sub.add_argument("--expect-database", required=True)
            sub.add_argument("--expected-source-sha256", required=True)
            sub.add_argument("--pg-dsn-env", default="ONEAPI_MIGRATION_PG_DSN")
    args = parser.parse_args(argv)
    try:
        path = source_path(args.source)
        if args.command == "snapshot":
            snapshot(path, args.output)
            return 0
        # Reserve the report before any target write; never overwrite earlier evidence.
        with private_output(args.report) as output, closing(open_source(path)) as db:
            fingerprint = file_hash(path)
            if args.command in ("import", "verify") and fingerprint != args.expected_source_sha256:
                raise MigrationError("Source SHA-256 differs from the reviewed backup.")
            db.execute("BEGIN")
            tables = read_schema(db, args.allow_timestamp_truncation)
            report = inspect(db, tables, fingerprint)
            assert_unchanged_source(path, fingerprint)
            if args.command in ("import", "verify"):
                with closing(connect_postgres(args.pg_dsn_env, args.expect_database, args.command == "import")) as connection:
                    if args.command == "import":
                        import_database(db, tables, report, connection, path, fingerprint)
                    else:
                        with connection.transaction():
                            connection.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
                            connection.execute("SET LOCAL TimeZone='UTC'")
                            verify_tables(connection, tables, report["tables"])
                            assert_unchanged_source(path, fingerprint)
                        report["verification"] = "all_source_tables_match"
            report["result"] = "complete"
            json.dump(report, output, indent=2, ensure_ascii=True)
            output.write("\n")
            output.flush()
            os.fsync(output.fileno())
        print(f"{args.command}=complete tables={len(tables)} source_sha256={fingerprint} application_blockers={len(report['application_blockers'])}", flush=True)
        return 0
    except MigrationError as error:
        print("migration=failed " + str(error), file=sys.stderr)
    except Exception as error:
        # psycopg errors may contain passwords, rejected row data, SQL, or DSNs.
        state = getattr(error, "sqlstate", None)
        code = state if isinstance(state, str) and re.fullmatch(r"[A-Z0-9]{5}", state) else "unavailable"
        print(f"migration=failed error_type={type(error).__name__} sqlstate={code}; details withheld to protect private data", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
