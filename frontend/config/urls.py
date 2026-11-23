from urllib.parse import urlencode

from django.http import Http404
from django.shortcuts import redirect, render
from django.urls import path
from django.views.decorators.http import require_GET, require_http_methods

import backend_client as backend
from pages import login, logout, account, backend_error

from backend_client import get_teacher_groups, get_teacher_students
from pages import (
    index, register, teacher_reviews, teacher_builder, teacher_calendar,
    student_calendar, student_comments, student_statistics, solve,
)
from learning_ui import assign_worksheet, teacher_assignments, student_assignments, start_attempt, save_answer, submit_attempt


@require_GET
def teacher_students(request):
    query = request.GET.get("q", "").strip()
    status = request.GET.get("status", "all")
    if status not in ["all", "active"]:
        status = "all"
    token = request.COOKIES.get("go_session", "")
    try:
        profile = backend.get_json("/api/v1/me", token)
        data = get_teacher_students(query, status, token)
    except backend.BackendError as error:
        return backend_error(request, error)

    selected_student = None
    selected_id = request.GET.get("student", "")
    for student in data["students"]:
        student["url"] = "?" + urlencode({"q": query, "status": status, "student": student["id"]})
        if str(student["id"]) == selected_id:
            selected_student = student
    if selected_id and selected_student is None:
        raise Http404("Ученик не найден в текущем списке")

    tabs = [
        {"key": "all", "label": "Все", "count": data["total"]},
        {"key": "active", "label": "Активные", "count": data["active_count"]},
    ]
    for tab in tabs:
        tab["url"] = "?" + urlencode({"q": query, "status": tab["key"]})
        tab["active"] = tab["key"] == status

    return render(request, "teacher_students.html", {
        "profile": profile, "live": True, "page_title": "Студенты", "active_page": "students",
        "students": data["students"], "total": data["total"],
        "query": query, "status": status, "tabs": tabs,
        "selected_student": selected_student,
        "close_url": "?" + urlencode({"q": query, "status": status}),
    })


@require_http_methods(["GET", "POST"])
def teacher_groups(request):
    query = request.GET.get("q", "").strip()
    status = request.GET.get("status", "all")
    if status not in ["all", "active", "archived"]:
        status = "all"
    token = request.COOKIES.get("go_session", "")
    error_message, response_status = "", 200
    if request.method == "POST":
        try:
            action = request.POST.get("action", "")
            group_id = request.POST.get("group_id", "")
            if action in ("update", "add", "remove"):
                if len(group_id) > 19 or not group_id.isascii() or not group_id.isdigit() or int(group_id) <= 0:
                    raise backend.BackendError(400, "Некорректный ID группы")
            path = "/api/v1/teacher/groups"
            if action in ("create", "update"):
                if action == "update":
                    path += "/" + group_id
                result = backend.get_json(path, token, "POST" if action == "create" else "PUT", {
                    "name": request.POST.get("name", ""),
                    "subject": request.POST.get("subject", "") or None,
                })
                group_id = result["id"]
            elif action in ("add", "remove"):
                student_id = request.POST.get("student_id", "")
                if len(student_id) > 19 or not student_id.isascii() or not student_id.isdigit() or int(student_id) <= 0:
                    raise backend.BackendError(400, "Некорректный ID ученика")
                path += "/" + group_id + "/members"
                if action == "add":
                    backend.get_json(path, token, "POST", {"student_id": int(student_id)})
                else:
                    backend.get_json(path + "/" + student_id, token, "DELETE")
            else:
                raise backend.BackendError(400, "Неизвестное действие")
            return redirect("/teacher/groups/?" + urlencode({"group": group_id}))
        except backend.BackendError as error:
            if error.status not in (400, 404, 409):
                return backend_error(request, error)
            error_message, response_status = error.message, error.status
    try:
        profile = backend.get_json("/api/v1/me", token)
        data = get_teacher_groups(query, status, token)
        students = get_teacher_students(token=token)["students"]
    except backend.BackendError as error:
        return backend_error(request, error)

    selected_group = None
    selected_id = request.GET.get("group", "")
    for group in data["groups"]:
        group["url"] = "?" + urlencode({"q": query, "status": status, "group": group["id"]})
        if str(group["id"]) == selected_id:
            selected_group = group
    if selected_id and selected_group is None:
        raise Http404("Группа не найдена в текущем списке")

    tabs = [
        {"key": "all", "label": "Все", "count": data["total"]},
        {"key": "active", "label": "Активные", "count": data["active_count"]},
        {"key": "archived", "label": "Архив", "count": data["archived_count"]},
    ]
    for tab in tabs:
        tab["url"] = "?" + urlencode({"q": query, "status": tab["key"]})
        tab["active"] = tab["key"] == status

    return render(request, "teacher_groups.html", {
        "profile": profile, "live": True, "page_title": "Группы", "active_page": "groups",
        "groups": data["groups"], "total": data["total"],
        "query": query, "status": status, "tabs": tabs,
        "selected_group": selected_group, "students": students,
        "error": error_message,
        "close_url": "?" + urlencode({"q": query, "status": status}),
    }, status=response_status)


urlpatterns = [
    path("login/", login),
    path("logout/", logout),
    path("", index),
    path("register/", register),
    path("teacher/", account),
    path("teacher/students/", teacher_students),
    path("teacher/groups/", teacher_groups),
    path("teacher/reviews/", teacher_reviews),
    path("teacher/builder/", teacher_builder),
    path("teacher/assignments/", teacher_assignments),
    path("teacher/assignments/issue/", assign_worksheet),
    path("teacher/calendar/", teacher_calendar),
    path("student/", account),
    path("student/calendar/", student_calendar),
    path("student/assignments/", student_assignments),
    path("student/assignments/<int:assignment>/start/", start_attempt),
    path("student/comments/", student_comments),
    path("student/statistics/", student_statistics),
    path("solve/", solve),
    path("solve/attempt/<int:attempt>/answer/<int:item>/", save_answer),
    path("solve/attempt/<int:attempt>/submit/", submit_attempt),
]
