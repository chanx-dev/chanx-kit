"""WebSocket consumers. After ``copit add``, list the kit's topics here."""

from dataclasses import dataclass
from typing import Any, ClassVar

from chanx.core.decorators import channel, ws_handler
from chanx.core.topic import Topic
from chanx.fast_channels.websocket import AsyncJsonWebsocketConsumer
from chanx.messages.incoming import PingMessage
from chanx.messages.outgoing import PongMessage
from chanx.utils.scope import query_params
from starlette.types import ASGIApp, Receive, Scope, Send

from app.layers import ALIAS
from app.ws_kits.notification import (
    BroadcastNotificationTopic,
    NotificationMessage,
    UserNotificationTopic,
)


@dataclass(frozen=True)
class User:
    id: str


class QueryAuth:
    """Demo identity from ``?as=<name>``, as the scope user. Replace with your auth."""

    def __init__(self, app: ASGIApp) -> None:
        self.app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if who := query_params(scope).get("as", [""])[0]:
            scope = {**scope, "user": User(id=who)}
        await self.app(scope, receive, send)


@channel(
    name="notifications",
    description="Notifications for the connected user, and for everyone.",
    tags=["notification"],
)
class NotificationConsumer(AsyncJsonWebsocketConsumer[NotificationMessage]):
    channel_layer_alias = ALIAS
    topics: ClassVar[list[type[Topic[Any]]]] = [
        UserNotificationTopic,
        BroadcastNotificationTopic,
    ]

    @ws_handler(summary="Ping", description="Connection health check.")
    async def handle_ping(self, _message: PingMessage) -> PongMessage:
        return PongMessage()
