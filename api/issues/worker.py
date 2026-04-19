"""Owner-approved Codex implementation worker."""

from __future__ import annotations

import textwrap
import uuid
from pathlib import Path
from typing import Optional

from .artifacts import ArtifactManager
from .codex_runner import CodexRunner
from .git_ops import GitOps
from .policy import PolicyChecker
from .store import IssueStore
from .verifier import Verifier


class WorkerError(RuntimeError):
    pass


class IssueWorker:
    WORKTREE_ROOT = Path(__file__).parent.parent.parent / "data" / "issue_worktrees"

    def __init__(
        self,
        store: IssueStore,
        repo_root: Optional[Path] = None,
        artifacts: Optional[ArtifactManager] = None,
        git_ops: Optional[GitOps] = None,
        codex_runner: Optional[CodexRunner] = None,
        policy: Optional[PolicyChecker] = None,
        verifier: Optional[Verifier] = None,
    ):
        self.store = store
        self.repo_root = Path(repo_root) if repo_root else Path(__file__).resolve().parents[2]
        self.artifacts = artifacts or ArtifactManager(store)
        self.git = git_ops or GitOps(self.repo_root)
        self.codex = codex_runner or CodexRunner()
        self.policy = policy or PolicyChecker()
        self.verifier = verifier or Verifier()

    def run(self, issue_id: int, requested_by: str) -> str:
        issue = self.store.ensure_issue(issue_id)
        if not issue:
            raise WorkerError(f"Report #{issue_id} not found")

        plan = self.store.get_latest_plan(int(issue["id"]))
        if not plan:
            raise WorkerError(f"Report #{issue_id} has no plan")
        if not plan.get("automation_safe"):
            raise WorkerError(f"Report #{issue_id} plan is not marked automation-safe")
        if not self.store.has_approval(int(issue["id"]), int(plan["id"]), str(plan["plan_hash"])):
            raise WorkerError(f"Report #{issue_id} does not have an approval for plan hash {plan['plan_hash']}")

        owner = f"worker-{uuid.uuid4().hex[:8]}"
        issue_scope = f"issue:{issue['id']}"
        repo_scope = "repo:LoloV6"
        if not self.store.acquire_lock(issue_scope, owner):
            raise WorkerError(f"Report #{issue_id} is already locked")
        if not self.store.acquire_lock(repo_scope, owner):
            self.store.release_lock(issue_scope, owner)
            raise WorkerError("Repository automation is already locked")

        run_id = self.store.create_run(int(issue["id"]), int(plan["id"]), str(plan["plan_hash"]))
        try:
            result = self._run_locked(issue, plan, run_id, requested_by)
            self.store.update_run(run_id, status="pr_open" if "PR:" in result else "review_pending")
            return result
        except Exception as exc:
            self.store.update_run(run_id, status="failed", finished_at=self._now(), exit_summary=str(exc))
            self.artifacts.write_text(
                int(issue["id"]),
                run_id,
                "worker-failure",
                str(exc),
                suffix=".txt",
                upload=True,
                summary="Worker failure",
            )
            raise
        finally:
            self.store.release_lock(issue_scope, owner)
            self.store.release_lock(repo_scope, owner)

    def _run_locked(self, issue: dict, plan: dict, run_id: int, requested_by: str) -> str:
        self.git.ensure_clean_tracked_state()
        base_sha = self.git.fetch_origin_main()
        branch, worktree = self.git.create_worktree(
            int(issue["id"]),
            str(issue.get("title") or f"issue-{issue['id']}"),
            base_sha,
            self.WORKTREE_ROOT,
            run_id=run_id,
        )
        self.store.update_run(
            run_id,
            base_remote="origin",
            base_branch="main",
            base_commit_sha=base_sha,
            branch_name=branch,
            worktree_path=str(worktree),
        )

        prompt = self._implementation_prompt(issue, plan, requested_by)
        self.artifacts.write_text(int(issue["id"]), run_id, "implementation-prompt", prompt, suffix=".md")
        final_message_path = self.artifacts.root / f"issue-{issue['id']}" / f"run-{run_id}" / "codex-final-message.md"
        codex_result = self.codex.run(
            worktree,
            prompt,
            final_message_path,
            should_cancel=lambda: self.store.is_cancel_requested(run_id),
        )
        self._raise_if_cancelled(run_id)
        self.git.cleanup_runtime_metadata(worktree)
        self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "codex-stdout-jsonl",
            str(codex_result.get("stdout") or ""),
            suffix=".jsonl",
        )
        self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "codex-stderr",
            str(codex_result.get("stderr") or ""),
            suffix=".txt",
        )
        self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "codex-final-message",
            str(codex_result.get("final_message") or ""),
            suffix=".md",
            upload=True,
            summary="Codex final message",
        )
        if int(codex_result.get("returncode") or 0) != 0:
            raise WorkerError("Codex implementation failed; see artifacts")

        policy_result = self.policy.check(worktree, planned_paths=plan.get("affected_paths") or [])
        self._raise_if_cancelled(run_id)
        _, _, policy_url, policy_upload_error = self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "policy-report",
            policy_result.render(),
            suffix=".txt",
            upload=True,
            summary="Policy report",
        )
        if not policy_result.ok:
            raise WorkerError(f"Policy check failed; report: {policy_url or policy_upload_error or 'local artifact'}")

        ok, verification_output = self.verifier.verify(worktree, policy_result.changed_files)
        verification_id, _, verification_url, verification_upload_error = self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "verification-output",
            verification_output,
            suffix=".txt",
            upload=True,
            summary="Verification output",
        )
        self.store.add_verification(
            int(issue["id"]),
            run_id,
            "verification",
            "go test ./... && go build ./cmd/bot && python -m py_compile changed-python",
            "passed" if ok else "failed",
            verification_id,
        )
        if not ok:
            raise WorkerError(f"Verification failed; output: {verification_url or verification_upload_error or 'local artifact'}")
        self._raise_if_cancelled(run_id)

        diff = self.git.diff(worktree)
        _, diff_path, diff_url, _ = self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "diff",
            diff,
            suffix=".patch",
            upload=True,
            summary="Implementation diff",
        )
        commit_hash = self.git.commit_all(worktree, f"issue #{issue['id']}: {issue.get('title', 'automation update')}")
        self.store.update_run(run_id, commit_hash=commit_hash)
        self._raise_if_cancelled(run_id)

        self.git.push(worktree, branch)
        pr_body = self._pr_body(issue, plan, diff_url, verification_url, policy_url)
        _, pr_body_path, _, _ = self.artifacts.write_text(
            int(issue["id"]),
            run_id,
            "pr-body",
            pr_body,
            suffix=".md",
            upload=True,
            summary="PR body",
        )
        pr_url = self.git.create_draft_pr(
            worktree,
            branch,
            f"Issue #{issue['id']}: {issue.get('title', 'automation update')}",
            pr_body_path,
        )
        self.store.update_run(run_id, pr_url=pr_url, finished_at=self._now(), exit_summary="Draft PR opened")
        return f"Report #{issue['id']} implementation finished. Branch {branch}. PR: {pr_url}. Diff: {diff_url or diff_path}"

    def _raise_if_cancelled(self, run_id: int) -> None:
        if self.store.is_cancel_requested(run_id):
            raise WorkerError("Run cancelled by owner request")

    def _implementation_prompt(self, issue: dict, plan: dict, requested_by: str) -> str:
        return textwrap.dedent(
            f"""
            Implement the approved plan for Lolo report #{issue['id']}.

            Requested by: {requested_by}
            Approved plan hash: {plan['plan_hash']}

            Issue title:
            {issue.get('title')}

            Issue description:
            {issue.get('description')}

            Approved plan:
            {plan.get('plan_text')}

            Hard rules:
            - Stay within the approved plan.
            - Do not edit secrets, .env files, live data, generated media, or deployment credentials.
            - Add or update focused tests where appropriate.
            - Do not merge branches.
            - Keep the final response concise and include what changed plus checks to run.
            """
        ).strip()

    def _pr_body(self, issue: dict, plan: dict, diff_url: Optional[str], verification_url: Optional[str], policy_url: Optional[str]) -> str:
        return textwrap.dedent(
            f"""
            Draft PR for report #{issue['id']}: {issue.get('title')}

            Plan hash: `{plan['plan_hash']}`

            Summary:
            {plan.get('summary')}

            Requested change:
            {plan.get('requested_change')}

            Verification:
            {verification_url or 'See local artifacts.'}

            Policy report:
            {policy_url or 'See local artifacts.'}

            Diff artifact:
            {diff_url or 'See PR diff.'}

            Human review required before merge.
            """
        ).strip()

    def _now(self) -> str:
        from .models import utc_now

        return utc_now()
