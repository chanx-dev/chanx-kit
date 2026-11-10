from django.contrib import admin
from django.urls import include, path, re_path
from django.views.static import serve

from app.views import notify

from config.settings import BASE_DIR

# Serves the built UI (`npm --prefix web run build`).
WEB_DIST = {"document_root": BASE_DIR / "web" / "dist"}

urlpatterns = [
    path("admin/", admin.site.urls),
    path("api/notify", notify),
    path("asyncapi/", include("chanx.channels.urls")),
    path("", serve, {"path": "index.html", **WEB_DIST}),
    re_path(r"^(?P<path>.+)$", serve, WEB_DIST),
]
