import sqlite3
import tempfile
import unittest
from pathlib import Path

from api.tools.ignore_vote import IgnoreVoteTool


class MutableClock:
    def __init__(self, timestamp: int = 1_700_000_000):
        self.timestamp = timestamp

    def __call__(self) -> float:
        return float(self.timestamp)


class IgnoreVoteToolTests(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.db_path = Path(self.temp_dir.name) / "bot.db"
        with sqlite3.connect(self.db_path) as connection:
            connection.execute(
                """
                CREATE TABLE users (
                    id INTEGER PRIMARY KEY AUTOINCREMENT,
                    nick TEXT NOT NULL UNIQUE,
                    hostmask TEXT NOT NULL,
                    level INTEGER NOT NULL,
                    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
                    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
                )
                """
            )
            connection.execute(
                """
                CREATE TABLE channel_users (
                    network TEXT NOT NULL,
                    channel TEXT NOT NULL,
                    nick TEXT NOT NULL,
                    hostmask TEXT
                )
                """
            )
        self.clock = MutableClock()
        self.tool = IgnoreVoteTool(db_path=str(self.db_path), now=self.clock)

    def tearDown(self):
        self.temp_dir.cleanup()

    def _add_user(self, nick: str, level: int) -> None:
        with sqlite3.connect(self.db_path) as connection:
            connection.execute(
                "INSERT INTO users(nick, hostmask, level) VALUES (?, '', ?)",
                (nick, level),
            )

    def _track_user(self, nick: str, hostmask: str) -> None:
        with sqlite3.connect(self.db_path) as connection:
            connection.execute(
                """
                INSERT INTO channel_users(network, channel, nick, hostmask)
                VALUES ('libera', '#test', ?, ?)
                """,
                (nick, hostmask),
            )

    def _level(self, nick: str):
        with sqlite3.connect(self.db_path) as connection:
            row = connection.execute(
                "SELECT level FROM users WHERE nick = ? COLLATE NOCASE", (nick,)
            ).fetchone()
        return None if row is None else row[0]

    def test_three_distinct_votes_ignore_an_unregistered_user(self):
        first = self.tool.execute(target_user="Trouble", requesting_user="Alice")
        second = self.tool.execute(target_user="trouble", requesting_user="Bob")
        third = self.tool.execute(target_user="TROUBLE", requesting_user="Carol")

        self.assertIn("1/3", first)
        self.assertIn("2/3", second)
        self.assertIn("added to the bot's ignore list", third)
        self.assertEqual(self._level("trouble"), 0)

    def test_duplicate_vote_does_not_increment_count(self):
        self.tool.execute(target_user="Target", requesting_user="Alice")

        duplicate = self.tool.execute(target_user="target", requesting_user="ALICE")
        next_vote = self.tool.execute(target_user="Target", requesting_user="Bob")

        self.assertIn("already voted", duplicate)
        self.assertIn("1/3", duplicate)
        self.assertIn("2/3", next_vote)

    def test_nick_change_on_same_hostmask_does_not_create_another_vote(self):
        self.tool.execute(
            target_user="Target",
            requesting_user="Alice",
            _voter_identity="ident@example.test",
        )

        result = self.tool.execute(
            target_user="Target",
            requesting_user="AliceAway",
            _voter_identity="ident@example.test",
        )

        self.assertIn("already voted", result)
        self.assertIn("1/3", result)

    def test_votes_at_least_24_hours_old_expire(self):
        self.tool.execute(target_user="Target", requesting_user="Alice")
        self.clock.timestamp += 24 * 60 * 60

        result = self.tool.execute(target_user="Target", requesting_user="Bob")

        self.assertIn("1/3", result)

    def test_existing_normal_user_is_changed_to_ignored(self):
        self._add_user("Target", 1)

        for voter in ("Alice", "Bob", "Carol"):
            result = self.tool.execute(target_user="target", requesting_user=voter)

        self.assertIn("added to the bot's ignore list", result)
        self.assertEqual(self._level("Target"), 0)

    def test_admin_and_owner_targets_are_immune(self):
        self._add_user("AdminUser", 2)
        self._add_user("OwnerUser", 3)

        admin_result = self.tool.execute(target_user="adminuser", requesting_user="Alice")
        owner_result = self.tool.execute(target_user="OWNERUSER", requesting_user="Alice")

        self.assertIn("cannot be voted", admin_result)
        self.assertIn("cannot be voted", owner_result)
        self.assertEqual(self._level("AdminUser"), 2)
        self.assertEqual(self._level("OwnerUser"), 3)

    def test_admin_using_an_alias_is_immune_by_tracked_hostmask(self):
        with sqlite3.connect(self.db_path) as connection:
            connection.execute(
                "INSERT INTO users(nick, hostmask, level) VALUES ('AdminUser', 'admin@host', 2)"
            )
        self._track_user("AdminAway", "admin@host")

        result = self.tool.execute(
            target_user="AdminAway",
            requesting_user="Alice",
            _current_network="libera",
            _current_channel="#test",
        )

        self.assertIn("cannot be voted", result)
        self.assertIsNone(self._level("AdminAway"))

    def test_self_vote_is_rejected(self):
        result = self.tool.execute(target_user="Alice", requesting_user="alice")

        self.assertEqual(result, "You cannot vote to ignore yourself.")

    def test_dynamically_authenticated_owner_nick_is_immune(self):
        result = self.tool.execute(target_user="Mathisen", requesting_user="Alice")

        self.assertIn("bot owner", result)


if __name__ == "__main__":
    unittest.main()
