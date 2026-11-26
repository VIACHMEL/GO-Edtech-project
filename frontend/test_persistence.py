import json
from unittest.mock import patch

from django.test import SimpleTestCase, Client, override_settings

import backend_client as backend


@override_settings(ALLOWED_HOSTS=["testserver"])
class PersistenceViewsTests(SimpleTestCase):
    def test_builder_restores_pinned_tasks_without_bank_results(self):
        draft = {"id": 12, "revision": 3, "title": "Saved draft", "max_attempts": 2,
                 "tasks": [{"id": 7, "task_version_id": 19, "content_text": "Pinned text"}]}
        with patch("backend_client.get_json", side_effect=[
            {"id": 4, "role": "teacher"}, [], draft,
            {"worksheets": [{"id": 12, "title": "Saved draft", "status": "draft"}]},
        ]) as api:
            response = self.client.get("/teacher/builder/?draft=12&subject=another")
        self.assertEqual(response.status_code, 200)
        self.assertEqual(response["Cache-Control"], "no-store")
        self.assertEqual(response.context["builder"]["selected_tasks"], draft["tasks"])
        self.assertContains(response, "Saved draft")
        self.assertIn("/api/v1/teacher/worksheets/12/draft", [c.args[0] for c in api.call_args_list])

    def test_draft_conflict_is_forwarded(self):
        payload = {"worksheet_id": 12, "expected_revision": 3, "title": "Changed", "items": []}
        self.client.cookies["go_session"] = "session-token"
        with patch("backend_client.get_json", side_effect=backend.BackendError(409, "Conflict")) as api:
            response = self.client.post("/teacher/builder/save/", json.dumps(payload), content_type="application/json")
        self.assertEqual(response.status_code, 409)
        self.assertEqual(response.json(), {"error": "Conflict"})
        api.assert_called_once_with("/api/v1/teacher/worksheets/12/draft", "session-token", "PUT",
                                    {k: v for k, v in payload.items() if k != "worksheet_id"})

    def test_new_draft_and_csrf(self):
        with patch("backend_client.get_json", return_value={"id": 12, "revision": 1}) as api:
            response = self.client.post("/teacher/builder/save/", "{}", content_type="application/json")
        self.assertEqual(response.json()["id"], 12)
        self.assertEqual(api.call_args.args[:3], ("/api/v1/teacher/worksheets/draft", "", "POST"))
        protected = Client(enforce_csrf_checks=True)
        self.assertEqual(protected.post("/teacher/builder/save/", "{}", content_type="application/json").status_code, 403)
        self.assertEqual(protected.post("/solve/attempt/1/answer/2/", "{}", content_type="application/json").status_code, 403)

    def test_bad_draft_payload_does_not_call_go(self):
        with patch("backend_client.get_json") as api:
            for payload in ("[]", "null", '{"worksheet_id":-1}', '{"worksheet_id":true}', "{"):
                response = self.client.post("/teacher/builder/save/", payload, content_type="application/json")
                self.assertEqual(response.status_code, 400)
            api.assert_not_called()

    def test_answer_save_preserves_private_photo_references(self):
        with patch("backend_client.get_json", side_effect=[
            [{"worksheet_item_id": 2, "answer_data": {"photos": ["private.png"]}}],
            {"worksheet_item_id": 2, "answer_revision": 4},
        ]) as api:
            response = self.client.post("/solve/attempt/1/answer/2/", json.dumps({
                "text": "42", "expected_revision": 3,
            }), content_type="application/json")
        self.assertEqual(response.status_code, 200)
        self.assertEqual(api.call_args.args, ("/api/v1/attempts/1/answers/2", "", "PUT", {
            "text": "42", "data": {"photos": ["private.png"]}, "expected_revision": 3,
        }))
