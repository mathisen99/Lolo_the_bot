"""Fixed upstream attribution and non-relicensing boundary."""

CREDITS_TEXT = (
    "Inspired by the mechanics of FSF Avenger; upstream program credits "
    "© 2026 Britney Lozza / CerberusGames.ca. Upstream Source TEMP-GAME.pas states GPLv3. "
    "This Standard profile uses independently authored fictionalized content. "
    "GPL-derived material remains under its applicable GPL terms; Lolo's root MIT license "
    "applies only to Lolo's original code and content. Lolo's MIT license does not relicense "
    "upstream material or GPL-derived material."
)


def render_credits() -> str:
    """Return attribution text verbatim; authored or AI content cannot replace it."""
    return CREDITS_TEXT


__all__ = ["CREDITS_TEXT", "render_credits"]
