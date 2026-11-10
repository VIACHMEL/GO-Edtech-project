"""Минимальные настройки только для локального запуска."""

from pathlib import Path

from django.core.management.utils import get_random_secret_key

BASE_DIR = Path(__file__).resolve().parent.parent

# Служебный ключ Django: новый при каждом запуске, не хранится в файлах.
SECRET_KEY = get_random_secret_key()

DEBUG = True
ALLOWED_HOSTS = ["localhost", "127.0.0.1", "[::1]"]
ROOT_URLCONF = "config.urls"
WSGI_APPLICATION = "config.wsgi.application"

INSTALLED_APPS = ["django.contrib.staticfiles"]
MIDDLEWARE = []
DATABASES = {}

TEMPLATES = [
    {
        "BACKEND": "django.template.backends.django.DjangoTemplates",
        "DIRS": [BASE_DIR / "templates"],
        "APP_DIRS": False,
    }
]
STATIC_URL = "/static/"
STATICFILES_DIRS = [BASE_DIR / "static"]
STATIC_ROOT = BASE_DIR / "staticfiles"

# Адрес будущего Go-сервера и время ожидания сетевой операции в секундах.
BACKEND_BASE_URL = "http://127.0.0.1:8080"
BACKEND_TIMEOUT = 3
