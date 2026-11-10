"""ASGI entry point: Django for HTTP, chanx consumers for WebSocket."""

import os

from django.core.asgi import get_asgi_application

os.environ.setdefault("DJANGO_SETTINGS_MODULE", "config.settings")
django_asgi_app = get_asgi_application()

from channels.routing import ProtocolTypeRouter, URLRouter  # noqa: E402

from app.consumers import QueryAuth  # noqa: E402
from app.routing import websocket_urlpatterns  # noqa: E402

application = ProtocolTypeRouter(
    {
        "http": django_asgi_app,
        "websocket": QueryAuth(URLRouter(websocket_urlpatterns)),
    }
)
