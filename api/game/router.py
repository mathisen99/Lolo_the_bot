"""Dedicated game routes; never dispatch through generic commands or mentions."""
from uuid import UUID

from fastapi import APIRouter, Header, HTTPException, status

from .models.api import (
    ErrorCategory, GameActionRequest, GameActionResponse, GameHealthResponse,
    LifecycleRequest, LifecycleResponse,
)
from .runtime import game_runtime

router = APIRouter(prefix="/game", tags=["game"])


def _require_matching_request_id(body_id: UUID, header_id: str | None) -> None:
    if header_id is None or header_id != str(body_id):
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={"category": ErrorCategory.REQUEST_ID_MISMATCH.value, "request_id": str(body_id)},
        )


def _validate_optional_health_request_id(header_id: str | None) -> None:
    if header_id is None:
        return
    try:
        UUID(header_id)
    except ValueError as exc:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail={"category": ErrorCategory.INVALID_INPUT.value, "field": "X-Request-ID"},
        ) from exc


@router.post("/action", response_model=GameActionResponse)
async def game_action(request: GameActionRequest, x_request_id: str | None = Header(default=None)) -> GameActionResponse:
    _require_matching_request_id(request.request_id, x_request_id)
    config = game_runtime.config()
    text = request.action.arguments.text
    if text is not None and len(text.encode()) > config.max_input_bytes:
        raise HTTPException(status_code=400, detail={"category": ErrorCategory.INVALID_INPUT.value, "field": "action.arguments.text"})
    response = await game_runtime.handle_action(request)
    if response.request_id != request.request_id:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail={"category": ErrorCategory.RESPONSE_INVALID.value, "request_id": str(request.request_id)},
        )
    return response


@router.post("/lifecycle", response_model=LifecycleResponse)
async def game_lifecycle(request: LifecycleRequest, x_request_id: str | None = Header(default=None)) -> LifecycleResponse:
    _require_matching_request_id(request.request_id, x_request_id)
    response = await game_runtime.handle_lifecycle(request)
    if response.request_id != request.request_id:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail={"category": ErrorCategory.RESPONSE_INVALID.value, "request_id": str(request.request_id)},
        )
    return response


@router.get("/health", response_model=GameHealthResponse)
async def game_health(x_request_id: str | None = Header(default=None)) -> GameHealthResponse:
    _validate_optional_health_request_id(x_request_id)
    return game_runtime.health()
