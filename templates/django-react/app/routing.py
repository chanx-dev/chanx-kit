from chanx.channels.routing import path

from app.consumers import NotificationConsumer

websocket_urlpatterns = [
    path("ws/notifications", NotificationConsumer.as_asgi()),
]
