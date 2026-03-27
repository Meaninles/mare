import os
import tempfile
import unittest

from cloud115_core import (
    build_upload_state_payload,
    load_upload_state_with_recovery,
    save_upload_state,
    upload_state_backup_path,
)


class _FakeUploader:
    def __init__(self, upload_id: str, url: str, callback: dict):
        self.upload_id = upload_id
        self.url = url
        self.callback = callback


class Cloud115UploadStateTests(unittest.TestCase):
    def setUp(self):
        self.temp_dir = tempfile.TemporaryDirectory()
        self.source_file = os.path.join(self.temp_dir.name, "sample.bin")
        with open(self.source_file, "wb") as handle:
            handle.write(b"mam-cloud115-test")
        self.state_path = os.path.join(self.temp_dir.name, "cloud115-upload-session.json")

    def tearDown(self):
        self.temp_dir.cleanup()

    def test_load_upload_state_restores_backup_when_primary_is_corrupted(self):
        save_upload_state(
            self.source_file,
            "projects/sample.bin",
            7,
            "sample.bin",
            _FakeUploader("upload-old", "https://example.invalid/old", {"token": "old"}),
            part_size=4096,
            resume_state_path=self.state_path,
        )
        save_upload_state(
            self.source_file,
            "projects/sample.bin",
            7,
            "sample.bin",
            _FakeUploader("upload-new", "https://example.invalid/new", {"token": "new"}),
            part_size=4096,
            resume_state_path=self.state_path,
        )

        self.assertTrue(os.path.exists(upload_state_backup_path(self.state_path)))
        with open(self.state_path, "w", encoding="utf-8") as handle:
            handle.write("{broken-json")

        payload, recovery = load_upload_state_with_recovery(
            self.source_file,
            "projects/sample.bin",
            7,
            "sample.bin",
            resume_state_path=self.state_path,
        )

        self.assertIsNotNone(payload)
        self.assertEqual("upload-old", payload["uploadId"])
        self.assertTrue(recovery["stateRecovered"])
        self.assertTrue(recovery["stateCorrupted"])
        self.assertEqual("backup", recovery["stateSource"])

    def test_load_upload_state_bootstraps_when_state_file_missing(self):
        bootstrap = build_upload_state_payload(
            self.source_file,
            "projects/sample.bin",
            7,
            "sample.bin",
            upload_url="https://example.invalid/bootstrap",
            callback={"token": "bootstrap"},
            upload_id="upload-bootstrap",
            part_size=8192,
        )

        payload, recovery = load_upload_state_with_recovery(
            self.source_file,
            "projects/sample.bin",
            7,
            "sample.bin",
            resume_state_path=self.state_path,
            bootstrap_state=bootstrap,
        )

        self.assertIsNotNone(payload)
        self.assertEqual("upload-bootstrap", payload["uploadId"])
        self.assertTrue(os.path.exists(self.state_path))
        self.assertTrue(recovery["stateRecovered"])
        self.assertEqual("bootstrap", recovery["stateSource"])


if __name__ == "__main__":
    unittest.main()
