"""Запросы Django к Go API. Доступа к БД здесь нет."""

import json
from urllib.error import HTTPError, URLError
from urllib.request import Request, build_opener, ProxyHandler, HTTPRedirectHandler

from django.conf import settings


class BackendError(Exception):
    def __init__(self, status, message):
        self.status = status
        self.message = message
        super().__init__(message)


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def get_json(path, token="", method="GET", data=None):
    if not path.startswith("/") or path.startswith("//"):
        raise ValueError("Ожидается путь с одним начальным /")
    headers = {"Accept": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    body = None
    if data is not None:
        body = json.dumps(data).encode("utf-8")
        headers["Content-Type"] = "application/json"
    request = Request(settings.BACKEND_BASE_URL.rstrip("/") + path,
                      data=body, headers=headers, method=method)
    try:
        opener = build_opener(ProxyHandler({}), NoRedirect())
        with opener.open(request, timeout=settings.BACKEND_TIMEOUT) as response:
            return json.load(response)
    except HTTPError as error:
        status = error.code if error.code in (400, 401, 403, 404, 409) else 503
        message = "Go backend недоступен"
        if status != 503:
            try:
                message = json.load(error)["error"]
            except (ValueError, UnicodeError, KeyError, TypeError):
                message = "Ошибка запроса"
        raise BackendError(status, message) from error
    except (URLError, OSError, ValueError, UnicodeError) as error:
        raise BackendError(503, "Go backend недоступен") from error


def get_teacher_students(query="", status="all", token=""):
    from urllib.parse import urlencode
    return get_json("/api/v1/teacher/students?" + urlencode({"q": query, "status": status}), token)


def get_teacher_groups(query="", status="all", token=""):
    from urllib.parse import urlencode
    return get_json("/api/v1/teacher/groups?" + urlencode({"q": query, "status": status}), token)


def get_teacher_builder(filters, token):
    from urllib.parse import urlencode
    return {
        "tasks": get_json("/api/v1/teacher/tasks?" + urlencode(filters), token),
        "draft_title": "", "selected_ids": [],
    }
