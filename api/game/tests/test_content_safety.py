from __future__ import annotations

import hashlib
import json
import shutil
from pathlib import Path

import pytest

try:
    import tomllib
except ModuleNotFoundError:  # Python 3.10
    import tomli as tomllib

from api.game.content import ContentValidationError, load_standard_campaign, load_standard_snapshot
from api.game.content.loader import EXPECTED_FILES
from api.game.renderer.credits import CREDITS_TEXT, render_credits

CONTENT_ROOT = Path(__file__).resolve().parents[1] / "content"
EXPECTED_CREDITS = (
    "Inspired by the mechanics of FSF Avenger; upstream program credits "
    "© 2026 Britney Lozza / CerberusGames.ca. Upstream Source TEMP-GAME.pas states GPLv3. "
    "This Standard profile uses independently authored fictionalized content. "
    "GPL-derived material remains under its applicable GPL terms; Lolo's root MIT license "
    "applies only to Lolo's original code and content. Lolo's MIT license does not relicense "
    "upstream material or GPL-derived material."
)


def _copy_content(tmp_path: Path) -> Path:
    target = tmp_path / "content"
    shutil.copytree(CONTENT_ROOT, target)
    return target


def _replace_manifest_hash(root: Path, name: str) -> None:
    path = root / "standard" / name
    digest = hashlib.sha256(path.read_bytes()).hexdigest()
    manifest_path = root / "standard" / "manifest.toml"
    document = manifest_path.read_text(encoding="utf-8")
    marker = f'path = "{name}"'
    start = document.index(marker)
    hash_start = document.index('sha256 = "', start) + len('sha256 = "')
    hash_end = document.index('"', hash_start)
    manifest_path.write_text(document[:hash_start] + digest + document[hash_end:], encoding="utf-8")


def test_standard_campaign_loads_complete_hashed_referentially_valid_content() -> None:
    campaign = load_standard_campaign()
    snapshot = load_standard_snapshot()

    assert campaign.version == "standard-2026.1"
    assert snapshot.version == campaign.version
    assert snapshot.profile == "standard"
    assert len(snapshot.manifest_hash) == 64
    assert dict(snapshot.records)["campaign"] == campaign
    assert {location.location_id for location in campaign.locations} == {
        "haven", "docks", "clinic", "archive", "spire",
    }
    assert campaign.final_location_id == "spire"
    assert campaign.final_table_id == "final_encounter"
    assert campaign.available_profiles == frozenset({"standard"})
    assert campaign.location("haven").description
    assert campaign.investigation("search_docks").fact
    assert all(1 <= len(table.outcomes) <= 16 for table in campaign.random_tables)


def test_every_standard_document_declares_original_fictionalized_safe_metadata() -> None:
    for name in EXPECTED_FILES:
        document = json.loads((CONTENT_ROOT / "standard" / name).read_text(encoding="utf-8"))
        assert document["schema_version"] == 1
        assert document["profile"] == "standard"
        assert document["classification"] == "original_content"
        assert document["fictionalized"] is True
        assert document["real_person_content"] is False
    text = json.loads((CONTENT_ROOT / "standard" / "text.json").read_text(encoding="utf-8"))
    assert text["prohibited_categories"] == []


def test_loader_rejects_hash_tampering_and_executable_content(tmp_path: Path) -> None:
    tampered = _copy_content(tmp_path / "hash")
    locations = tampered / "standard" / "locations.json"
    locations.write_text(locations.read_text(encoding="utf-8") + " ", encoding="utf-8")
    with pytest.raises(ContentValidationError, match="hash mismatch"):
        load_standard_campaign(tampered)

    executable = _copy_content(tmp_path / "executable")
    path = executable / "standard" / "locations.json"
    document = json.loads(path.read_text(encoding="utf-8"))
    document["script"] = "do_not_execute()"
    path.write_text(json.dumps(document), encoding="utf-8")
    _replace_manifest_hash(executable, "locations.json")
    with pytest.raises(ContentValidationError, match="executable field"):
        load_standard_campaign(executable)


def test_provenance_has_exactly_one_original_record_per_shipped_file() -> None:
    with (CONTENT_ROOT / "provenance.toml").open("rb") as handle:
        provenance = tomllib.load(handle)
    records = provenance["records"]
    expected = {"standard/manifest.toml", *(f"standard/{name}" for name in EXPECTED_FILES)}

    assert {record["path"] for record in records} == expected
    assert len(records) == len(expected)
    for record in records:
        assert record["classification"] == "original_content"
        assert record["adaptation_status"] == "independent_fictionalization"
        assert record["real_person_content"] is False
        assert record["product_approved"] is True
        assert record["provenance_approved"] is True
        assert record["license"] == "MIT"
        assert record["license_evidence"] == "repository root LICENSE"
    assert provenance["upstream_title"] == "FSF Avenger"
    assert provenance["upstream_source"] == "TEMP-GAME.pas"
    assert provenance["upstream_copyright_notice"] == "© 2026 Britney Lozza / CerberusGames.ca"
    assert provenance["upstream_stated_license"] == "GPLv3"
    assert "does not relicense" in provenance["license_boundary"]


def test_loader_refuses_unapproved_adapted_or_real_person_records(tmp_path: Path) -> None:
    adapted = _copy_content(tmp_path / "adapted")
    provenance_path = adapted / "provenance.toml"
    value = provenance_path.read_text(encoding="utf-8")
    value = value.replace('classification = "original_content"', 'classification = "adapted_content"', 1)
    value = value.replace("product_approved = true", "product_approved = false", 1)
    provenance_path.write_text(value, encoding="utf-8")
    with pytest.raises(ContentValidationError, match="unapproved Adapted Content"):
        load_standard_campaign(adapted)

    real_person = _copy_content(tmp_path / "real-person")
    provenance_path = real_person / "provenance.toml"
    value = provenance_path.read_text(encoding="utf-8").replace(
        "real_person_content = false", "real_person_content = true", 1,
    )
    provenance_path.write_text(value, encoding="utf-8")
    with pytest.raises(ContentValidationError, match="Real-Person Content"):
        load_standard_campaign(real_person)


def test_credits_renderer_is_fixed_and_preserves_exact_license_boundary() -> None:
    assert CREDITS_TEXT == EXPECTED_CREDITS
    assert render_credits() == EXPECTED_CREDITS
    assert "FSF Avenger" in CREDITS_TEXT
    assert "© 2026 Britney Lozza / CerberusGames.ca" in CREDITS_TEXT
    assert "TEMP-GAME.pas states GPLv3" in CREDITS_TEXT
    assert "independently authored fictionalized content" in CREDITS_TEXT
    assert "root MIT license applies only to Lolo's original code and content" in CREDITS_TEXT
    assert "MIT license does not relicense upstream material" in CREDITS_TEXT
