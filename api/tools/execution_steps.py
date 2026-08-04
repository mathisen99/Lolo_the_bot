"""Tool for showing a sanitized audit trail from the user's previous request."""

from typing import Any, Dict, Optional

from api.execution_trace import ExecutionTraceStore
from .base import Tool


class ExecutionStepsTool(Tool):
    def __init__(self, store: Optional[ExecutionTraceStore] = None) -> None:
        self.store = store or ExecutionTraceStore()

    @property
    def name(self) -> str:
        return "show_execution_steps"

    def get_definition(self) -> Dict[str, Any]:
        return {
            "type": "function",
            "name": self.name,
            "description": (
                "Show the sanitized action/tool audit trail for this user's most recent "
                "completed request in the current channel. Use when they ask how you got "
                "an answer or ask to see your steps. This does not expose private reasoning."
            ),
            "parameters": {
                "type": "object",
                "properties": {},
                "additionalProperties": False,
            },
        }

    def execute(self, **kwargs: Any) -> str:
        current_request_id = str(kwargs.get("_current_request_id", ""))
        self.store.mark_trace_lookup(current_request_id)
        trace = self.store.latest_trace(
            nick=str(kwargs.get("_requesting_user", "")),
            network=str(kwargs.get("_current_network", "libera")),
            channel=str(kwargs.get("_current_channel", "")),
            exclude_request_id=current_request_id,
        )
        if trace is None:
            return "No completed execution trace was found for your previous request in this channel."

        steps = trace["steps"]
        if not steps:
            return (
                "That answer did not use any recorded tools or multi-step actions. "
                "It was produced as a direct response."
            )

        rendered = []
        for index, step in enumerate(steps, start=1):
            suffix = "" if step["outcome"] == "completed" else f" ({step['outcome']})"
            rendered.append(f"{index}. {step['summary']}{suffix}")
        return "Sanitized execution steps (not private chain-of-thought): " + " ".join(rendered)
