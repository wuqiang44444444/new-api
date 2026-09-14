#!/usr/bin/env python3
"""Preview and repair missing SQLite log usernames; Python standard library only."""

import argparse
import csv
import hashlib
import os
from pathlib import Path
import sqlite3
import sys
import time


class RepairError(Exception):
    pass


def database_path(value):
    path = Path(value).expanduser().resolve(strict=True)
    if not path.is_file():
        raise RepairError("Database path must be an existing file.")
    return path


def connect(path, writable=False):
    # mode=rw prevents a typo from silently creating a new database.
    db = sqlite3.connect(path.as_uri() + ("?mode=rw" if writable else "?mode=ro"),
                         uri=True, timeout=10, isolation_level=None)
    db.execute("PRAGMA busy_timeout=10000")
    return db


def identity(path):
    path = path.resolve(strict=True)
    stat = path.stat()
    return hashlib.sha256(f"{path}\0{stat.st_dev}\0{stat.st_ino}".encode()).hexdigest()


def validate_schema(users, logs):
    users.execute("SELECT id, username FROM users LIMIT 0")
    columns = logs.execute("PRAGMA table_info(logs)").fetchall()
    if not {"id", "user_id", "created_at", "username"}.issubset({c[1] for c in columns}):
        raise RepairError("Missing required logs columns.")
    primary = [c[1] for c in columns if c[5]]
    if primary != ["id"]:
        raise RepairError("Expected logs.id to be the sole primary key.")
    # A trigger could turn a username-only UPDATE into unrelated data changes.
    if logs.execute("SELECT 1 FROM sqlite_master WHERE type='trigger' AND tbl_name='logs'").fetchone():
        raise RepairError("logs has triggers; review their side effects before using this tool.")


def preview(users, logs, main_path, log_path, plan_path, after, before, batch_size):
    maximum = logs.execute("SELECT COALESCE(MAX(id), 0) FROM logs").fetchone()[0]
    missing = planned = unresolved = cursor = 0
    # Exclusive creation prevents overwriting a previous repair plan.
    fd = os.open(plan_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w", newline="", encoding="utf-8") as output:
        writer = csv.writer(output)
        writer.writerow(["log-usernames-v1", identity(main_path), identity(log_path)])
        writer.writerow(["id", "user_id", "created_at", "old_is_null", "new_username"])
        while True:
            rows = logs.execute(
                "SELECT id, user_id, created_at, username FROM logs "
                "WHERE id > ? AND id <= ? AND created_at >= ? AND created_at < ? "
                "AND (username = '' OR username IS NULL) ORDER BY id LIMIT ?",
                (cursor, maximum, after, before, batch_size)).fetchall()
            if not rows:
                break
            cursor = rows[-1][0]
            user_ids = sorted({row[1] for row in rows if row[1] is not None and row[1] > 0})
            names = {}
            if user_ids:
                marks = ",".join("?" for _ in user_ids)
                # Includes soft-deleted users: the durable ID still identifies that account.
                names = dict(users.execute(
                    f"SELECT id, username FROM users WHERE id IN ({marks})", user_ids).fetchall())
            for row_id, user_id, created_at, old_name in rows:
                missing += 1
                name = names.get(user_id)
                if not isinstance(name, str) or not name.strip():
                    unresolved += 1
                    continue
                writer.writerow([row_id, user_id, created_at, int(old_name is None), name])
                planned += 1
        # A missing footer means an interrupted preview; apply refuses that file.
        writer.writerow(["END", planned])
        output.flush()
        os.fsync(output.fileno())
    print(f"mode=preview after={after} before={before} max_id={maximum} "
          f"missing={missing} planned={planned} unresolved={unresolved}", flush=True)
    return planned


def read_plan(plan_path, main_path, log_path):
    with open(plan_path, newline="", encoding="utf-8") as source:
        reader = csv.reader(source)
        if next(reader, None) != ["log-usernames-v1", identity(main_path), identity(log_path)]:
            raise RepairError("Plan database identity differs; generate a new preview for this database.")
        if next(reader, None) != ["id", "user_id", "created_at", "old_is_null", "new_username"]:
            raise RepairError("Invalid plan header.")
        changes = []
        last_id = 0
        for row in reader:
            if row and row[0] == "END":
                if row != ["END", str(len(changes))] or next(reader, None) is not None:
                    raise RepairError("Invalid plan footer.")
                return changes
            if len(row) != 5:
                raise RepairError("Invalid plan row.")
            row_id, user_id, created_at, old_null = map(int, row[:4])
            if row_id <= last_id or user_id <= 0 or created_at < 0 or old_null not in (0, 1) or not row[4].strip():
                raise RepairError("Invalid plan value or row order.")
            changes.append((row_id, user_id, created_at, old_null, row[4]))
            last_id = row_id
    raise RepairError("Incomplete plan; generate a new preview.")


def backup_database(logs, backup_path):
    fd = os.open(backup_path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    os.close(fd)
    target = sqlite3.connect(str(backup_path))
    try:
        # SQLite backup API includes committed WAL data; copying just the .db does not.
        logs.backup(target, pages=256)
        if target.execute("PRAGMA quick_check").fetchall() != [("ok",)]:
            raise RepairError("Backup integrity check failed; no repair was started.")
    finally:
        target.close()
    with open(backup_path, "rb") as saved:
        os.fsync(saved.fileno())
    print("backup=complete", flush=True)


def apply_plan(logs, changes, batch_size):
    updated = already = conflicts = 0
    for start in range(0, len(changes), batch_size):
        batch_updated = batch_already = batch_conflicts = 0
        logs.execute("BEGIN IMMEDIATE")
        try:
            for row_id, user_id, created_at, old_null, new_name in changes[start:start + batch_size]:
                current = logs.execute(
                    "SELECT user_id, created_at, username FROM logs WHERE id = ?", (row_id,)).fetchone()
                if current is None or current[:2] != (user_id, created_at):
                    batch_conflicts += 1
                    continue
                if current[2] == new_name:
                    batch_already += 1
                    continue
                old_name = None if old_null else ""
                if current[2] != old_name:
                    batch_conflicts += 1
                    continue
                result = logs.execute(
                    "UPDATE logs SET username = ? WHERE id = ? AND user_id = ? "
                    "AND created_at = ? AND username IS ?",
                    (new_name, row_id, user_id, created_at, old_name))
                if result.rowcount != 1:
                    raise RepairError("Unexpected update count; current batch rolled back.")
                batch_updated += 1
            logs.execute("COMMIT")
        except BaseException:
            if logs.in_transaction:
                logs.execute("ROLLBACK")
            raise
        updated += batch_updated
        already += batch_already
        conflicts += batch_conflicts
        print(f"mode=apply processed={min(start + batch_size, len(changes))}/{len(changes)} "
              f"updated={updated} already={already} conflicts={conflicts}", flush=True)
    print(f"result={'conflicts' if conflicts else 'ok'} updated={updated} "
          f"already={already} conflicts={conflicts}", flush=True)
    return conflicts


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--db", required=True, help="existing SQLite file containing users")
    parser.add_argument("--log-db", help="SQLite logs file; defaults to --db")
    parser.add_argument("--plan", required=True, help="new CSV for preview; existing CSV for apply")
    parser.add_argument("--apply", action="store_true", help="apply the existing plan (default: preview only)")
    parser.add_argument("--backup", help="new full logs-database backup file; required for --apply")
    parser.add_argument("--after", type=int, help="preview: inclusive Unix seconds; default 0")
    parser.add_argument("--before", type=int, help="preview: exclusive Unix seconds; default current time")
    parser.add_argument("--batch-size", type=int, default=200, help="1..500; default 200")
    args = parser.parse_args()
    if not 1 <= args.batch_size <= 500:
        raise RepairError("batch-size must be between 1 and 500.")
    if args.apply and (not args.backup or args.after is not None or args.before is not None):
        raise RepairError("Apply requires --backup and uses the plan's scope; omit --after/--before.")
    if not args.apply and args.backup:
        raise RepairError("--backup is only used with --apply.")
    after = args.after if args.after is not None else 0
    before = args.before if args.before is not None else int(time.time())
    if not args.apply and (after < 0 or before <= after):
        raise RepairError("Preview requires 0 <= after < before.")
    main_path = database_path(args.db)
    log_path = database_path(args.log_db or args.db)
    users = connect(main_path)
    logs = connect(log_path, writable=args.apply)
    try:
        validate_schema(users, logs)
        if args.apply:
            changes = read_plan(args.plan, main_path, log_path)
            backup_database(logs, args.backup)
            return 3 if apply_plan(logs, changes, args.batch_size) else 0
        preview(users, logs, main_path, log_path, args.plan, after, before, args.batch_size)
        return 0
    finally:
        users.close()
        logs.close()


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (RepairError, OSError, sqlite3.Error, ValueError, csv.Error) as error:
        # Driver errors and CSV values can contain private data; don't print them.
        message = str(error) if isinstance(error, RepairError) else type(error).__name__
        print(f"error={message}; stopped. Earlier committed batches may remain; "
              "keep the plan and backup. Never restore a live database by copying files.", file=sys.stderr)
        sys.exit(1)
    except KeyboardInterrupt:
        print("interrupted; current batch rolled back. Keep the plan and backup to resume.", file=sys.stderr)
        sys.exit(130)
