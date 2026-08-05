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
                "Show the detailed, sanitized action and evidence audit for this user's "
                "most recent completed request in the current channel. Use it to explain "
                "what was done, why each action was relevant, which searches, URLs and "
                "images were used, what the tools returned, and how that observable "
                "evidence supports the answer. This does not expose private reasoning."
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

        rendered = [
            "Detailed execution audit (observable actions and evidence; not private chain-of-thought):",
        ]
        if trace.get("objective"):
            rendered.append(f"\nOriginal request:\n{trace['objective']}")
        rendered.append(f"\nRun status: {trace['status']}")

        for index, step in enumerate(steps, start=1):
            suffix = "" if step["outcome"] == "completed" else f" ({step['outcome']})"
            tool = f" [{step['tool_name']}]" if step.get("tool_name") else ""
            rendered.append(f"\n{index}. {step['summary']}{tool}{suffix}")
            if step.get("details"):
                rendered.append(f"Input/purpose:\n{step['details']}")
            if step.get("result_summary"):
                rendered.append(f"Observed result/evidence:\n{step['result_summary']}")

        if trace.get("final_answer"):
            rendered.append(f"\nAnswer produced from the above evidence:\n{trace['final_answer']}")

        rendered.append(
            "\nExplain this audit clearly to the user, including why the actions were "
            "relevant and how the recorded evidence led to the answer. Preserve direct "
            "source and image URLs. If it is long, put the complete audit in a paste."
        )
        return "\n".join(rendered)
