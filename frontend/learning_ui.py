"""HTTP calls from Django forms to the existing Go learning API."""

import json
from uuid import UUID, uuid4

from django.http import HttpResponse, JsonResponse
from django.shortcuts import redirect, render
from django.views.decorators.http import require_GET, require_POST

import backend_client as backend


def create_worksheet(request, token):
    title = request.POST.get("title", "").strip()
    subject = request.POST.get("subject", "").strip()
    try:
        version_ids = json.loads(request.POST.get("version_ids", ""))
        max_attempts = int(request.POST.get("max_attempts", ""))
        if not isinstance(version_ids, list) or not 1 <= len(version_ids) <= 100:
            raise ValueError
        if any(type(value) is not int or not 0 < value < 2**63 for value in version_ids):
            raise ValueError
        if not 1 <= len(title) <= 120 or not 1 <= len(subject) <= 64 or not 1 <= max_attempts < 2**31:
            raise ValueError
    except (ValueError, TypeError):
        raise backend.BackendError(400, "Укажите название, лимит попыток и задания одного предмета.")

    path = "/api/v1/teacher/worksheets"
    draft = backend.get_json(path, token, "POST", {
        "title": title, "subject": subject, "max_attempts": max_attempts,
    })
    path += "/" + str(draft["id"])
    try:
        for position, version_id in enumerate(version_ids, start=1):
            backend.get_json(path + "/items", token, "POST", {
                "task_version_id": version_id, "position": position, "max_score": "1.00",
            })
        backend.get_json(path + "/publish", token, "POST")
    except backend.BackendError as error:
        raise backend.BackendError(error.status, f"Черновик №{draft['id']} создан. {error.message}") from error
    return draft["id"]


def builder_assignment_context(request, token):
    worksheet_id = request.GET.get("worksheet", "")
    if not worksheet_id.isascii() or not worksheet_id.isdecimal() or len(worksheet_id) > 19:
        return {}
    worksheet = backend.get_json(f"/api/v1/teacher/worksheets/{worksheet_id}", token)
    students = backend.get_teacher_students(token=token)["students"]
    return {"worksheet": worksheet, "students": students, "distribution": str(uuid4())}


def request_error(error):
    if error.status == 401:
        return redirect("/login/")
    return HttpResponse(error.message, status=error.status, content_type="text/plain; charset=utf-8")


@require_POST
def assign_worksheet(request):
    token = request.COOKIES.get("go_session", "")
    try:
        worksheet_id = int(request.POST.get("worksheet_id", ""))
        student_id = int(request.POST.get("student_id", ""))
        distribution = str(UUID(request.POST.get("distribution", "")))
    except ValueError:
        return HttpResponse("Выберите ученика и вариант.", status=400, content_type="text/plain; charset=utf-8")
    try:
        assignment = backend.get_json("/api/v1/teacher/assignments", token, "POST", {
            "worksheet_id": worksheet_id, "student_id": student_id, "distribution": distribution,
        })
    except backend.BackendError as error:
        return request_error(error)
    return redirect(f"/teacher/assignments/?assignment={assignment['id']}")


def assignment_list(request, role):
    token = request.COOKIES.get("go_session", "")
    try:
        profile = backend.get_json("/api/v1/me", token)
        assignments = backend.get_json(f"/api/v1/{role}/assignments", token)
        students = {}
        if role == "teacher":
            students = {s["id"]: s["name"] for s in backend.get_teacher_students(token=token)["students"]}
        selected = next((a for a in assignments if str(a["id"]) == request.GET.get("assignment")), None)
        attempts = backend.get_json(f"/api/v1/assignments/{selected['id']}/attempts", token) if selected else []
    except backend.BackendError as error:
        return request_error(error)
    for assignment in assignments:
        assignment["student_name"] = students.get(assignment["student_id"], "")
    base = f"/{role}/assignments/"
    title = "Назначенные работы" if role == "teacher" else "Задания и варианты"
    response = render(request, "live_assignments.html", {
        "profile": profile, "live": True, "base_template": f"{role}_base.html",
        "workspace_home": f"/{role}/", "workspace_role": "Преподаватель" if role == "teacher" else "Ученик",
        "page_title": title, "active_page": "assignments", "base_url": base,
        "assignments": assignments, "selected": selected, "attempts": attempts,
        "can_start": selected and selected["status"] == "active" and role == "student"
        and (any(a["status"] == "draft" for a in attempts) or len(attempts) < selected["max_attempts"]),
    })
    response["Cache-Control"] = "no-store"
    return response


@require_GET
def teacher_assignments(request):
    return assignment_list(request, "teacher")


@require_GET
def student_assignments(request):
    return assignment_list(request, "student")


@require_POST
def start_attempt(request, assignment):
    try:
        result = backend.get_json(
            f"/api/v1/assignments/{assignment}/start", request.COOKIES.get("go_session", ""), "POST",
        )
    except backend.BackendError as error:
        return request_error(error)
    return redirect(f"/solve/?attempt={result['id']}")


def go_json_response(call):
    try:
        return JsonResponse(call())
    except backend.BackendError as error:
        return JsonResponse({"error": error.message}, status=error.status)


@require_POST
def save_answer(request, attempt, item):
    try:
        payload = json.loads(request.body)
        text, revision = payload["text"], payload["expected_revision"]
        if not isinstance(text, str) or type(revision) is not int or revision < 0:
            raise ValueError
    except (KeyError, TypeError, ValueError):
        return JsonResponse({"error": "Некорректный ответ."}, status=400)
    token = request.COOKIES.get("go_session", "")
    path = f"/api/v1/attempts/{attempt}/answers"

    def save():
        answers = backend.get_json(path, token)
        saved_answer = next((a for a in answers if a["worksheet_item_id"] == item), None)
        if saved_answer is None:
            raise backend.BackendError(404, "Ответ не найден.")
        return backend.get_json(f"{path}/{item}", token, "PUT", {
            "text": text, "data": saved_answer.get("answer_data"), "expected_revision": revision,
        })

    return go_json_response(save)


@require_POST
def submit_attempt(request, attempt):
    return go_json_response(lambda: backend.get_json(
        f"/api/v1/attempts/{attempt}/submit", request.COOKIES.get("go_session", ""), "POST",
    ))
