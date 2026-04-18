"""Read-only Codex planner for issue reports."""

from __future__ import annotations

import json
import shutil
import subprocess
import tempfile
import textwrap
from pathlib import Path
from typing import Any, Dict, List, Optional

from .models import coerce_str_list, compute_plan_hash


PLAN_SCHEMA: Dict[str, Any] = {
    "type": "object",
    "additionalProperties": False,
    "required": [
        "summary",
        "issue_type",
        "requested_change",
        "affected_paths",
        "risk_level",
        "acceptance_criteria",
        "test_plan",
        "rollback_plan",
        "automation_safe",
        "plan_text",
    ],
    "properties": {
        "summary": {"type": "string"},
        "issue_type": {"type": "string", "enum": ["bug", "feature", "chore"]},
        "requested_change": {"type": "string"},
        "affected_paths": {"type": "array", "items": {"type": "string"}},
        "risk_level": {"type": "string", "enum": ["low", "medium", "high"]},
        "acceptance_criteria": {"type": "array", "items": {"type": "string"}},
        "test_plan": {"type": "array", "items": {"type": "string"}},
        "rollback_plan": {"type": "string"},
        "automation_safe": {"type": "boolean"},
        "plan_text": {"type": "string"},
    },
}


class PlannerError(RuntimeError):
    pass


class CodexPlanner:
    def __init__(self, repo_root: Optional[Path] = None, codex_bin: str = "codex", timeout: int = 300):
        self.repo_root = Path(repo_root) if repo_root else Path(__file__).resolve().parents[2]
        self.codex_bin = codex_bin
        self.timeout = timeout

    def probe(self) -> None:
        if not shutil.which(self.codex_bin):
            raise PlannerError(f"Codex CLI not found: {self.codex_bin}")

    def build_prompt(self, issue: Dict[str, Any], comments: List[Dict[str, Any]]) -> str:
        comments_text = "\n".join(
            f"- {item.get('created_at', '')} {item.get('author', '')}: {item.get('body', '')}"
            for item in comments
        )
        if not comments_text:
            comments_text = "- No comments"

        return textwrap.dedent(
            f"""
            You are planning a code change for the Lolo IRC bot repository.

            Produce a structured implementation plan only. Do not edit files.
            The plan must be safe for later automation and should be specific
            enough for a separate Codex run to implement after owner approval.

            Issue:
            - id: {issue.get('id')}
            - type: {issue.get('type')}
            - title: {issue.get('title')}
            - status: {issue.get('status')}
            - priority: {issue.get('priority')}
            - reporter: {issue.get('reporter')}
            - channel: {issue.get('channel')}

            Description:
            {issue.get('description')}

            Comments:
            {comments_text}

            Constraints:
            - Keep existing bug_report behavior compatible.
            - Do not propose auto-merge or deployment automation.
            - Prefer narrow changes and focused tests.
            - Mark automation_safe=false if the request is too vague, risky,
              cannot be verified, requires secrets, or needs human design input.
            """
        ).strip()

    def plan(self, issue: Dict[str, Any], comments: List[Dict[str, Any]]) -> Dict[str, Any]:
        self.probe()
        with tempfile.NamedTemporaryFile("w", encoding="utf-8", suffix=".json", delete=False) as schema_file:
            json.dump(PLAN_SCHEMA, schema_file)
            schema_path = schema_file.name

        prompt = self.build_prompt(issue, comments)
        cmd = [
            self.codex_bin,
            "exec",
            "--cd",
            str(self.repo_root),
            "--sandbox",
            "read-only",
            "--output-schema",
            schema_path,
            "-",
        ]
        try:
            result = subprocess.run(
                cmd,
                input=prompt,
                capture_output=True,
                text=True,
                timeout=self.timeout,
            )
        finally:
            try:
                Path(schema_path).unlink()
            except OSError:
                pass

        if result.returncode != 0:
            raise PlannerError(f"Codex planning failed: {result.stderr.strip() or result.stdout.strip()}")

        plan = self._parse_json(result.stdout.strip())
        return self.normalize_plan(plan)

    def _parse_json(self, output: str) -> Dict[str, Any]:
        try:
            return json.loads(output)
        except json.JSONDecodeError:
            start = output.find("{")
            end = output.rfind("}")
            if start >= 0 and end > start:
                try:
                    return json.loads(output[start : end + 1])
                except json.JSONDecodeError as exc:
                    raise PlannerError(f"Codex returned invalid JSON: {exc}") from exc
            raise PlannerError("Codex returned no JSON plan")

    def normalize_plan(self, plan: Dict[str, Any]) -> Dict[str, Any]:
        normalized = {
            "summary": str(plan.get("summary", "")).strip(),
            "issue_type": str(plan.get("issue_type", "bug")).strip().lower(),
            "requested_change": str(plan.get("requested_change", "")).strip(),
            "affected_paths": coerce_str_list(plan.get("affected_paths")),
            "risk_level": str(plan.get("risk_level", "medium")).strip().lower(),
            "acceptance_criteria": coerce_str_list(plan.get("acceptance_criteria")),
            "test_plan": coerce_str_list(plan.get("test_plan")),
            "rollback_plan": str(plan.get("rollback_plan", "")).strip(),
            "automation_safe": bool(plan.get("automation_safe", False)),
            "plan_text": str(plan.get("plan_text", "")).strip(),
        }
        if normalized["issue_type"] not in {"bug", "feature", "chore"}:
            normalized["issue_type"] = "bug"
        if normalized["risk_level"] not in {"low", "medium", "high"}:
            normalized["risk_level"] = "medium"
        if not normalized["plan_text"]:
            normalized["plan_text"] = self.render_plan_text(normalized)
        normalized["plan_hash"] = compute_plan_hash(normalized)
        return normalized

    def render_plan_text(self, plan: Dict[str, Any]) -> str:
        def lines(items: List[str]) -> str:
            return "\n".join(f"- {item}" for item in items) or "- None"

        return textwrap.dedent(
            f"""
            # {plan.get('summary', 'Implementation plan')}

            Requested change:
            {plan.get('requested_change', '')}

            Affected paths:
            {lines(plan.get('affected_paths') or [])}

            Acceptance criteria:
            {lines(plan.get('acceptance_criteria') or [])}

            Test plan:
            {lines(plan.get('test_plan') or [])}

            Rollback:
            {plan.get('rollback_plan', '')}

            Automation safe: {plan.get('automation_safe')}
            Risk: {plan.get('risk_level')}
            """
        ).strip()
