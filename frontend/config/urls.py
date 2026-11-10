from urllib.parse import urlencode

from django.http import Http404
from django.shortcuts import render
from django.urls import path

from backend_client import get_teacher_groups, get_teacher_overview, get_teacher_students
from pages import (
    index, register, teacher_reviews, teacher_builder, teacher_calendar,
    student, student_calendar, student_assignments, student_comments, student_statistics, solve,
)


def teacher(request):
    overview = get_teacher_overview()
    return render(request, "teacher.html", {
        "overview": overview,
        "profile": overview["teacher"],
        "page_title": "Обзор",
        "active_page": "overview",
    })


def teacher_students(request):
    query = request.GET.get("q", "").strip()
    status = request.GET.get("status", "all")
    if status not in ["all", "active", "attention"]:
        status = "all"
    data = get_teacher_students(query, status)

    selected_student = None
    selected_id = request.GET.get("student", "")
    for student in data["students"]:
        student["url"] = "?" + urlencode({"q": query, "status": status, "student": student["id"]})
        if student["id"] == selected_id:
            selected_student = student
    if selected_id and selected_student is None:
        raise Http404("Ученик не найден в текущем списке")

    tabs = [
        {"key": "all", "label": "Все", "count": data["total"]},
        {"key": "active", "label": "Активные", "count": data["active_count"]},
        {"key": "attention", "label": "Требуют внимания", "count": data["attention_count"]},
    ]
    for tab in tabs:
        tab["url"] = "?" + urlencode({"q": query, "status": tab["key"]})
        tab["active"] = tab["key"] == status

    return render(request, "teacher_students.html", {
        "profile": data["teacher"], "page_title": "Студенты", "active_page": "students",
        "students": data["students"], "total": data["total"],
        "query": query, "status": status, "tabs": tabs,
        "selected_student": selected_student,
        "close_url": "?" + urlencode({"q": query, "status": status}),
    })


def teacher_groups(request):
    query = request.GET.get("q", "").strip()
    status = request.GET.get("status", "all")
    if status not in ["all", "active", "archived"]:
        status = "all"
    data = get_teacher_groups(query, status)

    selected_group = None
    selected_id = request.GET.get("group", "")
    for group in data["groups"]:
        group["url"] = "?" + urlencode({"q": query, "status": status, "group": group["id"]})
        if group["id"] == selected_id:
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
        "profile": data["teacher"], "page_title": "Группы", "active_page": "groups",
        "groups": data["groups"], "total": data["total"],
        "query": query, "status": status, "tabs": tabs,
        "selected_group": selected_group,
        "close_url": "?" + urlencode({"q": query, "status": status}),
    })


urlpatterns = [
    path("", index),
    path("register/", register),
    path("teacher/", teacher),
    path("teacher/students/", teacher_students),
    path("teacher/groups/", teacher_groups),
    path("teacher/reviews/", teacher_reviews),
    path("teacher/builder/", teacher_builder),
    path("teacher/calendar/", teacher_calendar),
    path("student/", student),
    path("student/calendar/", student_calendar),
    path("student/assignments/", student_assignments),
    path("student/comments/", student_comments),
    path("student/statistics/", student_statistics),
    path("solve/", solve),
]
