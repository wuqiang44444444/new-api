import importlib.util
import sqlite3
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).with_name("repair-log-usernames.py")
SPEC = importlib.util.spec_from_file_location("repair_log_usernames", SCRIPT)
repair = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(repair)


class RepairLogUsernamesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.main_path = self.root / "main.db"
        self.log_path = self.root / "logs.db"
        self.plan = self.root / "plan.csv"
        self.backup = self.root / "before.db"
        self.users = sqlite3.connect(self.main_path)
        self.logs = sqlite3.connect(self.log_path)
        self.addCleanup(self.users.close)
        self.addCleanup(self.logs.close)
        self.users.execute("CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, deleted_at TEXT)")
        self.users.executemany("INSERT INTO users VALUES (?, ?, ?)", [
            (1, "alice", None), (2, "逗号,引号\"换行\n用户", "2026-01-01"), (3, "", None)])
        self.users.commit()
        self.logs.execute("PRAGMA journal_mode=WAL")
        self.logs.execute("CREATE TABLE logs (id INTEGER PRIMARY KEY, user_id INTEGER, "
                          "created_at INTEGER, username TEXT, quota INTEGER, prompt_tokens INTEGER, "
                          "completion_tokens INTEGER, other TEXT)")
        self.logs.executemany("INSERT INTO logs VALUES (?, ?, ?, ?, ?, ?, ?, ?)", [
            (1, 1, 10, "", 7, 11, 12, "unchanged"),
            (2, 2, 10, None, 8, 13, 14, "unchanged"),
            (3, 1, 10, "historic-name", 9, 15, 16, "unchanged"),
            (4, 99, 10, "", 10, 17, 18, "unchanged"),
            (5, 0, 10, "", 11, 19, 20, "unchanged"),
            (6, 3, 10, "", 12, 21, 22, "unchanged"),
            (7, 1, 30, "", 13, 23, 24, "out-of-scope")])
        self.logs.commit()

    def run_script(self, *args, expected=0):
        result = subprocess.run([sys.executable, str(SCRIPT), "--db", str(self.main_path),
                                 "--log-db", str(self.log_path), "--plan", str(self.plan),
                                 "--batch-size", "1", *args], capture_output=True, text=True)
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        return result.stdout + result.stderr

    def snapshot(self):
        return self.logs.execute("SELECT * FROM logs ORDER BY id").fetchall()

    def test_preview_backup_apply_and_repeat_preserve_all_other_facts(self):
        before = self.snapshot()
        output = self.run_script("--before", "20")
        self.assertIn("missing=5 planned=2 unresolved=3", output)
        self.assertEqual(self.snapshot(), before)
        output = self.run_script("--apply", "--backup", str(self.backup))
        self.assertIn("updated=2 already=0 conflicts=0", output)
        saved = sqlite3.connect(self.backup)
        try:
            self.assertEqual(saved.execute("SELECT * FROM logs ORDER BY id").fetchall(), before)
        finally:
            saved.close()
        after = self.snapshot()
        self.assertEqual(after[0][3], "alice")
        self.assertEqual(after[1][3], '逗号,引号"换行\n用户')
        self.assertEqual(after[2:], before[2:])
        self.assertEqual([r[:3] + r[4:] for r in after], [r[:3] + r[4:] for r in before])
        output = self.run_script("--apply", "--backup", str(self.root / "repeat.db"))
        self.assertIn("updated=0 already=2 conflicts=0", output)
        self.assertEqual(self.snapshot(), after)

    def test_changes_after_preview_are_not_overwritten_or_added_to_scope(self):
        self.run_script("--before", "20")
        self.logs.execute("UPDATE logs SET username='manual' WHERE id=1")
        self.logs.execute("INSERT INTO logs VALUES (8, 1, 10, '', 1, 2, 3, 'new')")
        self.logs.commit()
        output = self.run_script("--apply", "--backup", str(self.backup), expected=3)
        self.assertIn("updated=1 already=0 conflicts=1", output)
        names = dict(self.logs.execute("SELECT id, username FROM logs"))
        self.assertEqual(names[1], "manual")
        self.assertEqual(names[8], "")

    def test_partial_execution_rolls_back_current_batch_and_can_resume(self):
        self.run_script("--before", "20")
        changes = repair.read_plan(self.plan, self.main_path, self.log_path)
        # Abort the second update to prove that a batch failure does not keep
        # the first update from the same transaction.
        self.logs.execute("CREATE TRIGGER fail_update BEFORE UPDATE ON logs WHEN NEW.id=2 "
                          "BEGIN SELECT RAISE(ABORT, 'test'); END")
        self.logs.commit()
        before = self.snapshot()
        with self.assertRaises(sqlite3.Error):
            repair.apply_plan(self.logs, changes, 2)
        self.assertEqual(self.snapshot(), before)
        with self.assertRaises(sqlite3.Error):
            repair.apply_plan(self.logs, changes, 1)
        self.assertEqual(self.snapshot()[0][3], "alice")
        self.logs.execute("DROP TRIGGER fail_update")
        self.logs.commit()
        output = self.run_script("--apply", "--backup", str(self.backup))
        self.assertIn("updated=1 already=1 conflicts=0", output)

    def test_incomplete_plan_and_existing_backup_are_rejected_without_writes(self):
        self.run_script("--before", "20")
        before = self.snapshot()
        complete = self.plan.read_text()
        self.plan.write_text(complete[:complete.rfind("END,")])
        self.run_script("--apply", "--backup", str(self.backup), expected=1)
        self.assertFalse(self.backup.exists())
        self.assertEqual(self.snapshot(), before)
        self.plan.write_text(complete)
        self.backup.write_bytes(b"preserve-existing-backup")
        self.run_script("--apply", "--backup", str(self.backup), expected=1)
        self.assertEqual(self.backup.read_bytes(), b"preserve-existing-backup")
        self.assertEqual(self.snapshot(), before)

    def test_wrong_database_and_unexpected_triggers_are_rejected(self):
        self.run_script("--before", "20")
        other = self.root / "other.db"
        target = sqlite3.connect(other)
        self.logs.backup(target)
        target.close()
        self.run_script("--log-db", str(other), "--apply", "--backup", str(self.backup), expected=1)
        self.assertFalse(self.backup.exists())
        self.logs.execute("CREATE TRIGGER touch_logs AFTER UPDATE ON logs BEGIN SELECT 1; END")
        self.logs.commit()
        self.run_script("--apply", "--backup", str(self.backup), expected=1)
        self.assertFalse(self.backup.exists())

    def test_same_database_is_supported_without_log_db_argument(self):
        self.users.execute("CREATE TABLE logs (id INTEGER PRIMARY KEY, user_id INTEGER, "
                           "created_at INTEGER, username TEXT)")
        self.users.execute("INSERT INTO logs VALUES (1, 1, 10, '')")
        self.users.commit()
        command = [sys.executable, str(SCRIPT), "--db", str(self.main_path), "--plan", str(self.plan)]
        result = subprocess.run(command + ["--before", "20"], capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        result = subprocess.run(command + ["--apply", "--backup", str(self.backup)], capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.users.execute("SELECT username FROM logs").fetchone(), ("alice",))


if __name__ == "__main__":
    unittest.main()
