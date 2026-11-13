"""Negative controls for the gate's fail-closed event accounting."""
import unittest
import json
import io
from contextlib import redirect_stdout, redirect_stderr
from unittest.mock import patch
import fast_ci
import sys
import tempfile
from pathlib import Path
from fast_ci import run, validate, select_suites


class TestReceipts(unittest.TestCase):
    def events(self):
        """events returns a minimal successful Go event stream."""
        return [{"Package": "p", "Test": "TestRequired", "Action": action} for action in ("run", "pass")] + [{"Package": "p", "Action": "pass"}]

    def test_success(self):
        """test_success accepts complete successful test accounting."""
        validate(self.events(), ["TestRequired"], 0)

    def test_empty_or_missing_tests(self):
        """test_empty_or_missing_tests rejects empty allowlists and missing tests."""
        for events, expected in (([], ["TestRequired"]), (self.events(), []), (self.events(), ["TestMissing"])):
            with self.assertRaises(RuntimeError):
                validate(events, expected, 0)

    def test_failed_process_even_with_pass_events(self):
        """test_failed_process_even_with_pass_events rejects failed native exits despite pass events."""
        with self.assertRaises(RuntimeError):
            validate(self.events(), ["TestRequired"], 1)

    def test_skipped_or_failed_subtest(self):
        """test_skipped_or_failed_subtest rejects unsuccessful nested test events."""
        for action in ("skip", "fail", "build-fail"):
            with self.assertRaises(RuntimeError):
                validate(self.events() + [{"Package": "p", "Test": "TestRequired/child", "Action": action}], ["TestRequired"], 0)

    def test_missing_package_completion(self):
        """test_missing_package_completion rejects missing package success."""
        with self.assertRaises(RuntimeError):
            validate(self.events()[:-1], ["TestRequired"], 0)

    def test_duplicate_pass(self):
        """test_duplicate_pass rejects duplicate run or pass accounting."""
        with self.assertRaises(RuntimeError):
            validate(self.events() + self.events()[:2], ["TestRequired"], 0)

    def test_native_failure_is_retained(self):
        """test_native_failure_is_retained retains nonzero native exits and raw output files."""
        records = []
        with tempfile.TemporaryDirectory() as directory:
            evidence = Path(directory)
            with self.assertRaises(RuntimeError):
                run([sys.executable, "-c", "import sys; sys.exit(7)"], evidence, "failure", records)
            self.assertEqual(records[0]["exit_code"], 7)
            self.assertTrue((evidence / "failure.stderr").is_file())

    def test_timeout_does_not_report_success(self):
        """test_timeout_does_not_report_success retains unknown native status for timed-out processes."""
        records = []
        with tempfile.TemporaryDirectory() as directory:
            with self.assertRaises(RuntimeError):
                run([sys.executable, "-c", "import time; time.sleep(10)"], Path(directory), "timeout", records, timeout=0.1)
            self.assertIsNone(records[0]["exit_code"])
            self.assertIn("timeout", records[0]["error"])


class TestPlatforms(unittest.TestCase):
    """TestPlatforms checks full configuration validation and target-specific accounting."""

    def config(self):
        """config returns portable tests, an optional test and a platform-only package."""
        return [
            {"package": "./portable", "tests": ["TestPortable", {"name": "TestLinux", "goos": ["linux"]}]},
            {"package": "./linux", "goos": ["linux"], "tests": ["TestOnlyLinux"]},
        ]

    def select(self, suites, goos="linux"):
        """select applies the fixture's known Go target set."""
        return select_suites(suites, goos, {"linux", "darwin", "windows"})

    def test_linux_keeps_every_applicable_test(self):
        """test_linux_keeps_every_applicable_test preserves portable and Linux coverage."""
        self.assertEqual(self.select(self.config()), [
            {"package": "./portable", "tests": ["TestPortable", "TestLinux"]},
            {"package": "./linux", "tests": ["TestOnlyLinux"]},
        ])

    def test_other_platforms_omit_only_inapplicable_tests(self):
        """test_other_platforms_omit_only_inapplicable_tests retains portable coverage."""
        for goos in ("darwin", "windows"):
            self.assertEqual(self.select(self.config(), goos), [{"package": "./portable", "tests": ["TestPortable"]}])

    def test_group_and_test_selectors_intersect(self):
        """test_group_and_test_selectors_intersect requires both selectors to apply."""
        config = self.config()
        config[0]["goos"] = ["darwin"]
        self.assertEqual(self.select(config, "darwin"), [{"package": "./portable", "tests": ["TestPortable"]}])

    def test_invalid_selectors_are_rejected_even_when_excluded(self):
        """test_invalid_selectors_are_rejected_even_when_excluded validates the whole file."""
        for value in ([], "linux", None, ["linxu"], ["linux", "linux"], [1], [True]):
            for group_level in (False, True):
                config = self.config()
                node = config[1] if group_level else config[0]["tests"][1]
                node["goos"] = value
                with self.assertRaises(RuntimeError):
                    self.select(config, "darwin")

    def test_duplicate_tests_are_rejected_across_platforms(self):
        """test_duplicate_tests_are_rejected_across_platforms rejects disjoint duplicate entries."""
        config = self.config()
        config.append({"package": "./portable", "goos": ["windows"], "tests": ["TestPortable"]})
        with self.assertRaisesRegex(RuntimeError, "duplicate configured test"):
            self.select(config)

    def test_same_name_in_different_packages_is_valid(self):
        """test_same_name_in_different_packages_is_valid preserves package-scoped names."""
        config = [{"package": "./a", "tests": ["TestSame"]}, {"package": "./b", "tests": ["TestSame"]}]
        self.assertEqual(self.select(config), config)

    def test_empty_configuration_or_selection_fails(self):
        """test_empty_configuration_or_selection_fails prevents a vacuous successful gate."""
        for config in ([], [{"package": "./a", "tests": []}], [{"package": "./a", "goos": ["linux"], "tests": ["TestA"]}]):
            with self.assertRaises(RuntimeError):
                self.select(config, "windows")

    def test_malformed_excluded_entries_fail(self):
        """test_malformed_excluded_entries_fail rejects names and misspelled selector keys."""
        for entry in ({"name": "TestA", "go_oss": ["linux"]}, {"name": "notATest"}, {"name": 7}, None):
            config = self.config()
            config[1]["tests"] = [entry]
            with self.assertRaises(RuntimeError):
                self.select(config, "darwin")

    def test_malformed_suite_is_rejected(self):
        """test_malformed_suite_is_rejected rejects invalid packages and misspelled group selectors."""
        for suite in (None, {"package": "./a", "tests": ["TestA"], "go_oss": ["linux"]},
                      {"package": "", "tests": ["TestA"]}, {"package": "-bad", "tests": ["TestA"]}):
            with self.assertRaises(RuntimeError):
                self.select([suite])

    def test_unknown_target_fails(self):
        """test_unknown_target_fails rejects an unrecognized execution target."""
        with self.assertRaises(RuntimeError):
            self.select(self.config(), "linxu")

    def exercise_main(self, goos, misspelled=False):
        """exercise_main runs the real gate orchestration against bounded native fixtures."""
        config = self.config()
        if misspelled:
            config[0]["tests"][0] = "TestPortablTypo"
        calls = []

        def native(command, evidence, label, records, timeout=240):
            """native supplies discovery and Go event output while recording selected commands."""
            calls.append(command)
            records.append({"label": label, "command": command, "exit_code": 0})
            if label == "source":
                return "fixture-commit\n"
            if label == "toolchain":
                return "go version fixture\n"
            if label == "tracked-go":
                return "fixture.go\0"
            if label == "goos":
                return goos + "\n"
            if label == "supported-platforms":
                return "linux/amd64\ndarwin/arm64\nwindows/amd64\n"
            if label.startswith("discover-"):
                return "TestPortable\n"
            if label.startswith("tests-"):
                return "\n".join(json.dumps(event) for event in [
                    {"Package": "p", "Test": "TestPortable", "Action": "run"},
                    {"Package": "p", "Test": "TestPortable", "Action": "pass"},
                    {"Package": "p", "Action": "pass"},
                ])
            return ""

        with tempfile.TemporaryDirectory() as directory:
            with patch.object(sys, "argv", ["fast_ci.py", "--evidence", directory]), \
                    patch("fast_ci.Path.read_text", return_value=json.dumps(config)), \
                    patch("fast_ci.run", side_effect=native), redirect_stdout(io.StringIO()), redirect_stderr(io.StringIO()):
                code = fast_ci.main()
            receipt = json.loads((Path(directory) / "receipt.json").read_text())
        return code, receipt, calls

    def test_main_does_not_discover_or_credit_excluded_tests(self):
        """test_main_does_not_discover_or_credit_excluded_tests accounts only for applicable names."""
        for goos in ("darwin", "windows"):
            code, receipt, calls = self.exercise_main(goos)
            self.assertEqual(code, 0)
            self.assertEqual(receipt["goos"], goos)
            self.assertEqual(receipt["selected_suites"], [{"package": "./portable", "tests": ["TestPortable"]}])
            test_calls = [command for command in calls if command[:2] == ["go", "test"]]
            self.assertEqual(len(test_calls), 2)
            self.assertTrue(all(command[-1] == "./portable" for command in test_calls))
            self.assertFalse(any("TestLinux" in argument for command in test_calls for argument in command))

    def test_main_applicable_discovery_typo_still_fails(self):
        """test_main_applicable_discovery_typo_still_fails rejects missing applicable names."""
        code, receipt, calls = self.exercise_main("darwin", misspelled=True)
        self.assertEqual(code, 1)
        self.assertFalse(receipt["passed"])
        self.assertIn("test discovery mismatch", receipt["error"])
        self.assertFalse(any("-json" in command for command in calls))


if __name__ == "__main__":
    unittest.main()
