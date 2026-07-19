"""Strict loader for the immutable, fictionalized Standard campaign corpus."""
from __future__ import annotations

import hashlib
import json
import re
from collections.abc import Mapping, Sequence
from pathlib import Path
from typing import Any

try:
    import tomllib
except ModuleNotFoundError:  # Python 3.10
    import tomli as tomllib

from ..engine.campaign import (
    CampaignContent,
    DayEvent,
    Encounter,
    Gate,
    Investigation,
    ItemDefinition,
    Location,
    ProgressionThreshold,
    QuestImplication,
    RandomOutcome,
    RandomTable,
    TravelEdge,
)

STANDARD_PROFILE = "standard"
SCHEMA_VERSION = 1
MAX_CONTENT_FILE_BYTES = 256 * 1024
MAX_TABLES = 64
MAX_OUTCOMES_PER_TABLE = 16
EXPECTED_FILES = frozenset({
    "locations.json", "encounters.json", "items.json", "quests.json",
    "random_tables.json", "text.json",
})
ID_PATTERN = re.compile(r"^[a-z0-9][a-z0-9_.:-]{0,63}$")
HASH_PATTERN = re.compile(r"^[0-9a-f]{64}$")
FORBIDDEN_EXECUTABLE_KEYS = frozenset({
    "script", "code", "executable", "command", "shell", "python", "module",
    "import", "callable", "callback", "eval", "exec", "template_engine",
})


class ContentValidationError(ValueError):
    """A shipped content pack failed a closed schema or safety check."""


def _exact(value: Mapping[str, Any], required: set[str], optional: set[str] = set(), *, where: str) -> None:
    missing = required - set(value)
    extra = set(value) - required - optional
    if missing or extra:
        raise ContentValidationError(f"{where} has invalid fields (missing={sorted(missing)}, extra={sorted(extra)})")


def _id(value: Any, *, where: str) -> str:
    if not isinstance(value, str) or not ID_PATTERN.fullmatch(value):
        raise ContentValidationError(f"{where} must be a bounded identifier")
    return value


def _integer(value: Any, minimum: int, maximum: int, *, where: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or not minimum <= value <= maximum:
        raise ContentValidationError(f"{where} must be between {minimum} and {maximum}")
    return value


def _bool(value: Any, *, where: str) -> bool:
    if not isinstance(value, bool):
        raise ContentValidationError(f"{where} must be boolean")
    return value


def _list(value: Any, *, where: str) -> list[Any]:
    if not isinstance(value, list):
        raise ContentValidationError(f"{where} must be an array")
    return value


def _unique(values: Sequence[str], *, where: str) -> None:
    if len(values) != len(set(values)):
        raise ContentValidationError(f"{where} contains duplicate identifiers")


def _reject_executable(value: Any, *, where: str = "content") -> None:
    if isinstance(value, Mapping):
        for key, child in value.items():
            if not isinstance(key, str):
                raise ContentValidationError(f"{where} has a non-string key")
            normalized = key.casefold().replace("-", "_")
            if normalized in FORBIDDEN_EXECUTABLE_KEYS or normalized.startswith("__"):
                raise ContentValidationError(f"{where} contains forbidden executable field {key!r}")
            _reject_executable(child, where=f"{where}.{key}")
    elif isinstance(value, list):
        for index, child in enumerate(value):
            _reject_executable(child, where=f"{where}[{index}]")
    elif not isinstance(value, (str, int, float, bool, type(None))):
        raise ContentValidationError(f"{where} contains a non-data value")


def _document_metadata(
    document: Mapping[str, Any],
    payload_key: str,
    *,
    filename: str,
    additional_fields: set[str] | None = None,
) -> None:
    _exact(
        document,
        {"schema_version", "profile", "classification", "fictionalized", "real_person_content", payload_key},
        {"equipment_slots", "prohibited_categories"} | (additional_fields or set()),
        where=filename,
    )
    if document["schema_version"] != SCHEMA_VERSION:
        raise ContentValidationError(f"{filename} schema version is unsupported")
    if document["profile"] != STANDARD_PROFILE:
        raise ContentValidationError(f"{filename} is not Standard profile content")
    if document["classification"] != "original_content":
        raise ContentValidationError(f"{filename} is not approved Original Content")
    if document["fictionalized"] is not True or document["real_person_content"] is not False:
        raise ContentValidationError(f"{filename} must be fictionalized and exclude Real-Person Content")


def _read_bytes(path: Path) -> bytes:
    if path.is_symlink() or not path.is_file():
        raise ContentValidationError(f"content path must be a regular file: {path.name}")
    data = path.read_bytes()
    if len(data) > MAX_CONTENT_FILE_BYTES:
        raise ContentValidationError(f"content file exceeds size limit: {path.name}")
    return data


def _read_json(path: Path, expected_hash: str) -> Mapping[str, Any]:
    data = _read_bytes(path)
    if hashlib.sha256(data).hexdigest() != expected_hash:
        raise ContentValidationError(f"immutable content hash mismatch: {path.name}")
    try:
        value = json.loads(data)
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise ContentValidationError(f"invalid JSON content: {path.name}") from exc
    if not isinstance(value, Mapping):
        raise ContentValidationError(f"content document must be an object: {path.name}")
    _reject_executable(value, where=path.name)
    return value


def _load_manifest(root: Path) -> tuple[Mapping[str, Any], dict[str, str], str]:
    path = root / "manifest.toml"
    data = _read_bytes(path)
    try:
        manifest = tomllib.loads(data.decode("utf-8"))
    except (UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
        raise ContentValidationError("invalid Standard manifest") from exc
    _exact(manifest, {
        "schema_version", "content_version", "campaign_id", "profile", "effective_profile",
        "classification", "fictionalized", "real_person_content", "immutable", "files",
    }, where="manifest.toml")
    if (
        manifest["schema_version"] != SCHEMA_VERSION
        or manifest["profile"] != STANDARD_PROFILE
        or manifest["effective_profile"] != STANDARD_PROFILE
        or manifest["classification"] != "original_content"
        or manifest["fictionalized"] is not True
        or manifest["real_person_content"] is not False
        or manifest["immutable"] is not True
    ):
        raise ContentValidationError("manifest does not declare the restrictive Standard fictionalized profile")
    _id(manifest["campaign_id"], where="manifest.campaign_id")
    version = manifest["content_version"]
    if not isinstance(version, str) or not re.fullmatch(r"[a-z0-9][a-z0-9.-]{0,63}", version):
        raise ContentValidationError("manifest content_version is invalid")
    hashes: dict[str, str] = {}
    for index, entry in enumerate(_list(manifest["files"], where="manifest.files")):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("manifest file entry must be an object")
        _exact(entry, {"path", "sha256"}, where=f"manifest.files[{index}]")
        name, digest = entry["path"], entry["sha256"]
        if name not in EXPECTED_FILES or not isinstance(digest, str) or not HASH_PATTERN.fullmatch(digest):
            raise ContentValidationError("manifest contains an unsupported path or hash")
        if name in hashes:
            raise ContentValidationError("manifest contains a duplicate file path")
        hashes[name] = digest
    if set(hashes) != EXPECTED_FILES:
        raise ContentValidationError("manifest file inventory is incomplete")
    return manifest, hashes, hashlib.sha256(data).hexdigest()


def _load_provenance(content_root: Path, shipped_paths: set[str]) -> None:
    path = content_root / "provenance.toml"
    data = _read_bytes(path)
    try:
        document = tomllib.loads(data.decode("utf-8"))
    except (UnicodeDecodeError, tomllib.TOMLDecodeError) as exc:
        raise ContentValidationError("invalid provenance manifest") from exc
    _exact(document, {
        "schema_version", "scope", "upstream_title", "upstream_source",
        "upstream_copyright_notice", "upstream_stated_license", "reuse_strategy",
        "license_boundary", "records",
    }, where="provenance.toml")
    if document["schema_version"] != SCHEMA_VERSION:
        raise ContentValidationError("provenance schema version is unsupported")
    if document["upstream_title"] != "FSF Avenger" or document["upstream_source"] != "TEMP-GAME.pas":
        raise ContentValidationError("required upstream attribution is missing")
    if document["upstream_copyright_notice"] != "© 2026 Britney Lozza / CerberusGames.ca":
        raise ContentValidationError("required upstream copyright notice is missing")
    if document["upstream_stated_license"] != "GPLv3":
        raise ContentValidationError("required stated GPLv3 status is missing")
    if document["reuse_strategy"] != "mechanics_only_independent_fictionalization":
        raise ContentValidationError("unapproved content reuse strategy")
    boundary = document["license_boundary"]
    if not isinstance(boundary, str) or "does not relicense" not in boundary or "GPL" not in boundary or "MIT" not in boundary:
        raise ContentValidationError("provenance license boundary is incomplete")
    records: dict[str, Mapping[str, Any]] = {}
    required = {
        "path", "classification", "origin", "author", "copyright_notice", "license",
        "license_evidence", "mechanics_reference", "adaptation_status", "real_person_content",
        "product_approved", "provenance_approved", "notes",
    }
    for index, record in enumerate(_list(document["records"], where="provenance.records")):
        if not isinstance(record, Mapping):
            raise ContentValidationError("provenance record must be an object")
        _exact(record, required, where=f"provenance.records[{index}]")
        record_path = record["path"]
        if not isinstance(record_path, str) or record_path in records:
            raise ContentValidationError("provenance paths must be unique strings")
        if record["classification"] == "adapted_content" and not (
            record["product_approved"] is True and record["provenance_approved"] is True
        ):
            raise ContentValidationError("unapproved Adapted Content is excluded")
        if record["classification"] != "original_content" or record["adaptation_status"] != "independent_fictionalization":
            raise ContentValidationError("Standard shipped files must be independently authored Original Content")
        if record["real_person_content"] is not False:
            raise ContentValidationError("Standard shipped files must exclude Real-Person Content")
        if record["license"] != "MIT" or record["license_evidence"] != "repository root LICENSE":
            raise ContentValidationError("Original Content license evidence is incomplete")
        records[record_path] = record
    if set(records) != shipped_paths:
        raise ContentValidationError("provenance must contain exactly one record per shipped file")


def _gate(value: Any, *, where: str) -> Gate:
    if not isinstance(value, Mapping):
        raise ContentValidationError(f"{where} must be an object")
    _exact(value, set(), {"required_flags", "required_items", "required_location", "minimum_level", "profile"}, where=where)
    flags = tuple(_id(item, where=f"{where}.required_flags") for item in _list(value.get("required_flags", []), where=f"{where}.required_flags"))
    _unique(flags, where=f"{where}.required_flags")
    required_items = []
    for item in _list(value.get("required_items", []), where=f"{where}.required_items"):
        if not isinstance(item, list) or len(item) != 2:
            raise ContentValidationError(f"{where}.required_items entry is invalid")
        required_items.append((_id(item[0], where=where), _integer(item[1], 1, 99, where=where)))
    profile = value.get("profile")
    if profile not in (None, STANDARD_PROFILE):
        raise ContentValidationError(f"{where} requests an unavailable profile")
    return Gate(
        required_flags=frozenset(flags),
        required_items=tuple(required_items),
        required_location=_id(value["required_location"], where=where) if value.get("required_location") is not None else None,
        minimum_level=_integer(value.get("minimum_level", 1), 1, 100, where=where),
        profile=profile,
    )


def _parse_text(document: Mapping[str, Any]) -> dict[str, str]:
    _document_metadata(document, "entries", filename="text.json")
    if document.get("prohibited_categories") != []:
        raise ContentValidationError("Standard text declares prohibited categories")
    result: dict[str, str] = {}
    for index, entry in enumerate(_list(document["entries"], where="text.entries")):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("text entry must be an object")
        _exact(entry, {"id", "text"}, where=f"text.entries[{index}]")
        text_id = _id(entry["id"], where="text.id")
        text = entry["text"]
        if not isinstance(text, str) or len(text.encode("utf-8")) > 600 or any(ord(char) < 32 and char not in "\t" for char in text):
            raise ContentValidationError(f"text entry {text_id} is invalid or oversized")
        if text_id in result:
            raise ContentValidationError("text contains duplicate identifiers")
        result[text_id] = text
    return result


def _parse_campaign(documents: Mapping[str, Mapping[str, Any]], version: str) -> CampaignContent:
    text = _parse_text(documents["text.json"])

    locations_doc = documents["locations.json"]
    _document_metadata(locations_doc, "locations", filename="locations.json")
    locations: list[Location] = []
    description_refs: list[str] = []
    for index, entry in enumerate(_list(locations_doc["locations"], where="locations")):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("location must be an object")
        _exact(entry, {"id", "name", "description_text_id", "recovery_allowed", "commerce_allowed", "investigation_id", "edges"}, where=f"locations[{index}]")
        edges: list[TravelEdge] = []
        for edge_index, edge in enumerate(_list(entry["edges"], where="location.edges")):
            if not isinstance(edge, Mapping):
                raise ContentValidationError("travel edge must be an object")
            _exact(edge, {"destination_id", "gate"}, where=f"locations[{index}].edges[{edge_index}]")
            edges.append(TravelEdge(_id(edge["destination_id"], where="edge.destination_id"), _gate(edge["gate"], where="edge.gate")))
        name = entry["name"]
        if not isinstance(name, str) or not 1 <= len(name) <= 64:
            raise ContentValidationError("location name is invalid")
        description_id = _id(entry["description_text_id"], where="location.description_text_id")
        description_refs.append(description_id)
        investigation_id = entry["investigation_id"]
        locations.append(Location(
            _id(entry["id"], where="location.id"), tuple(edges),
            _bool(entry["recovery_allowed"], where="location.recovery_allowed"),
            _bool(entry["commerce_allowed"], where="location.commerce_allowed"),
            _id(investigation_id, where="location.investigation_id") if investigation_id is not None else None,
            name, text.get(description_id, ""),
        ))

    items_doc = documents["items.json"]
    _document_metadata(items_doc, "items", filename="items.json")
    slots = tuple(_id(item, where="equipment_slots") for item in _list(items_doc["equipment_slots"], where="equipment_slots"))
    _unique(slots, where="equipment_slots")
    items: list[ItemDefinition] = []
    item_required = {"id", "name", "price", "sell_price", "capacity_cost", "recovery_amount", "equipment_slot", "combat_damage_bonus", "combat_usable"}
    for index, entry in enumerate(_list(items_doc["items"], where="items")):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("item must be an object")
        _exact(entry, item_required, where=f"items[{index}]")
        name = entry["name"]
        slot = entry["equipment_slot"]
        if not isinstance(name, str) or not 1 <= len(name) <= 64 or (slot is not None and slot not in slots):
            raise ContentValidationError("item presentation or equipment slot is invalid")
        items.append(ItemDefinition(
            _id(entry["id"], where="item.id"),
            _integer(entry["price"], 0, 1_000_000, where="item.price"),
            _integer(entry["sell_price"], 0, 1_000_000, where="item.sell_price"),
            _integer(entry["capacity_cost"], 1, 99, where="item.capacity_cost"),
            _integer(entry["recovery_amount"], 0, 1000, where="item.recovery_amount"),
            slot,
            _integer(entry["combat_damage_bonus"], 0, 1000, where="item.combat_damage_bonus"),
            _bool(entry["combat_usable"], where="item.combat_usable"),
            name,
        ))

    quests_doc = documents["quests.json"]
    _document_metadata(
        quests_doc,
        "investigations",
        filename="quests.json",
        additional_fields={
            "progression_thresholds", "day_events", "quest_implications", "finale",
            "known_abilities", "known_effects",
        },
    )
    _exact(quests_doc, {"schema_version", "profile", "classification", "fictionalized", "real_person_content", "investigations", "progression_thresholds", "day_events", "quest_implications", "finale", "known_abilities", "known_effects"}, where="quests.json")
    investigations: list[Investigation] = []
    investigation_text_refs: list[str] = []
    for entry in _list(quests_doc["investigations"], where="investigations"):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("investigation must be an object")
        _exact(entry, {"id", "location_id", "grants_flag", "grant_id", "experience", "reward_table_id", "fact_text_id"}, where="investigation")
        text_id = _id(entry["fact_text_id"], where="investigation.fact_text_id")
        investigation_text_refs.append(text_id)
        reward = entry["reward_table_id"]
        investigations.append(Investigation(
            _id(entry["id"], where="investigation.id"), _id(entry["location_id"], where="investigation.location_id"),
            _id(entry["grants_flag"], where="investigation.grants_flag"), _id(entry["grant_id"], where="investigation.grant_id"),
            _integer(entry["experience"], 0, 1_000_000, where="investigation.experience"),
            _id(reward, where="investigation.reward_table_id") if reward is not None else None,
            text.get(text_id, ""),
        ))
    thresholds: list[ProgressionThreshold] = []
    threshold_fields = {"level", "experience_required", "grant_id", "max_health_increase", "health_increase", "ability_id", "currency", "item_id", "item_quantity"}
    for entry in _list(quests_doc["progression_thresholds"], where="progression_thresholds"):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("progression threshold must be an object")
        _exact(entry, threshold_fields, where="progression_threshold")
        thresholds.append(ProgressionThreshold(
            _integer(entry["level"], 2, 100, where="threshold.level"),
            _integer(entry["experience_required"], 1, 1_000_000, where="threshold.experience"),
            _id(entry["grant_id"], where="threshold.grant_id"),
            _integer(entry["max_health_increase"], 0, 1000, where="threshold.max_health"),
            _integer(entry["health_increase"], 0, 1000, where="threshold.health"),
            _id(entry["ability_id"], where="threshold.ability") if entry["ability_id"] is not None else None,
            _integer(entry["currency"], 0, 1_000_000, where="threshold.currency"),
            _id(entry["item_id"], where="threshold.item") if entry["item_id"] is not None else None,
            _integer(entry["item_quantity"], 0, 99, where="threshold.quantity"),
        ))
    day_events: list[DayEvent] = []
    day_text_refs: list[str] = []
    for entry in _list(quests_doc["day_events"], where="day_events"):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("day event must be an object")
        _exact(entry, {"id", "day", "grants_flag", "fact_text_id"}, where="day_event")
        text_id = _id(entry["fact_text_id"], where="day_event.fact_text_id")
        day_text_refs.append(text_id)
        day_events.append(DayEvent(_id(entry["id"], where="day_event.id"), _integer(entry["day"], 1, 1_000_000, where="day_event.day"), _id(entry["grants_flag"], where="day_event.flag"), text.get(text_id, "")))
    implications: list[QuestImplication] = []
    for entry in _list(quests_doc["quest_implications"], where="quest_implications"):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("quest implication must be an object")
        _exact(entry, {"consequence_flag", "required_flags"}, where="quest_implication")
        flags = tuple(_id(flag, where="quest_implication.required_flags") for flag in _list(entry["required_flags"], where="quest_implication.required_flags"))
        implications.append(QuestImplication(_id(entry["consequence_flag"], where="quest_implication.consequence_flag"), frozenset(flags)))
    finale = quests_doc["finale"]
    if not isinstance(finale, Mapping):
        raise ContentValidationError("finale must be an object")
    _exact(finale, {"location_id", "table_id", "gate"}, where="finale")
    abilities = tuple(_id(item, where="known_abilities") for item in _list(quests_doc["known_abilities"], where="known_abilities"))
    effects = tuple(_id(item, where="known_effects") for item in _list(quests_doc["known_effects"], where="known_effects"))

    tables_doc = documents["random_tables.json"]
    _document_metadata(tables_doc, "tables", filename="random_tables.json")
    raw_tables = _list(tables_doc["tables"], where="tables")
    if not 1 <= len(raw_tables) <= MAX_TABLES:
        raise ContentValidationError("random table count is outside bounds")
    tables: list[RandomTable] = []
    outcome_text_refs: list[str] = []
    outcome_fields = {"id", "text_id"}
    outcome_optional = {"value", "currency", "item_id", "item_quantity", "experience", "grants_flag", "encounter_id", "grant_id", "completes_campaign"}
    for table_entry in raw_tables:
        if not isinstance(table_entry, Mapping):
            raise ContentValidationError("random table must be an object")
        _exact(table_entry, {"id", "version", "outcomes"}, where="random_table")
        raw_outcomes = _list(table_entry["outcomes"], where="random_table.outcomes")
        if not 1 <= len(raw_outcomes) <= MAX_OUTCOMES_PER_TABLE:
            raise ContentValidationError("random table outcome count is outside bounds")
        outcomes: list[RandomOutcome] = []
        for entry in raw_outcomes:
            if not isinstance(entry, Mapping):
                raise ContentValidationError("random outcome must be an object")
            _exact(entry, outcome_fields, outcome_optional, where="random_outcome")
            text_id = _id(entry["text_id"], where="random_outcome.text_id")
            outcome_text_refs.append(text_id)
            outcomes.append(RandomOutcome(
                _id(entry["id"], where="random_outcome.id"), text.get(text_id, ""),
                _integer(entry.get("value", 0), 0, 1000, where="random_outcome.value"),
                _integer(entry.get("currency", 0), 0, 1_000_000, where="random_outcome.currency"),
                _id(entry["item_id"], where="random_outcome.item_id") if entry.get("item_id") is not None else None,
                _integer(entry.get("item_quantity", 0), 0, 99, where="random_outcome.item_quantity"),
                _integer(entry.get("experience", 0), 0, 1_000_000, where="random_outcome.experience"),
                _id(entry["grants_flag"], where="random_outcome.grants_flag") if entry.get("grants_flag") is not None else None,
                _id(entry["encounter_id"], where="random_outcome.encounter_id") if entry.get("encounter_id") is not None else None,
                _id(entry["grant_id"], where="random_outcome.grant_id") if entry.get("grant_id") is not None else None,
                _bool(entry.get("completes_campaign", False), where="random_outcome.completes_campaign"),
            ))
        outcome_ids = [outcome.outcome_id for outcome in outcomes]
        _unique(outcome_ids, where="random table outcomes")
        table_version = table_entry["version"]
        if not isinstance(table_version, str) or not 1 <= len(table_version) <= 32:
            raise ContentValidationError("random table version is invalid")
        tables.append(RandomTable(_id(table_entry["id"], where="random_table.id"), table_version, tuple(outcomes)))

    encounters_doc = documents["encounters.json"]
    _document_metadata(encounters_doc, "encounters", filename="encounters.json")
    encounters: list[Encounter] = []
    encounter_fields = {"id", "name", "version", "enemy_max_health", "player_hit_table_id", "player_damage_table_id", "enemy_hit_table_id", "enemy_damage_table_id", "escape_table_id", "reward_table_id", "defeat_location_id", "defeat_health", "defeat_currency_loss", "defend_reduction", "completes_campaign"}
    for entry in _list(encounters_doc["encounters"], where="encounters"):
        if not isinstance(entry, Mapping):
            raise ContentValidationError("encounter must be an object")
        _exact(entry, encounter_fields, where="encounter")
        name = entry["name"]
        version_value = entry["version"]
        if not isinstance(name, str) or not 1 <= len(name) <= 64 or not isinstance(version_value, str) or not version_value:
            raise ContentValidationError("encounter presentation is invalid")
        encounters.append(Encounter(
            _id(entry["id"], where="encounter.id"), version_value,
            _integer(entry["enemy_max_health"], 1, 1000, where="encounter.health"),
            *(_id(entry[key], where=f"encounter.{key}") for key in ("player_hit_table_id", "player_damage_table_id", "enemy_hit_table_id", "enemy_damage_table_id", "escape_table_id", "reward_table_id", "defeat_location_id")),
            _integer(entry["defeat_health"], 1, 1000, where="encounter.defeat_health"),
            _integer(entry["defeat_currency_loss"], 0, 1_000_000, where="encounter.currency_loss"),
            _integer(entry["defend_reduction"], 0, 1000, where="encounter.defend_reduction"),
            _bool(entry["completes_campaign"], where="encounter.completes_campaign"),
            name,
        ))

    all_text_refs = description_refs + investigation_text_refs + day_text_refs + outcome_text_refs
    if any(reference not in text for reference in all_text_refs):
        raise ContentValidationError("content references unknown authored text")
    location_ids = [item.location_id for item in locations]
    item_ids = [item.item_id for item in items]
    encounter_ids = [item.encounter_id for item in encounters]
    table_ids = [item.table_id for item in tables]
    investigation_ids = [item.investigation_id for item in investigations]
    for values, where in ((location_ids, "locations"), (item_ids, "items"), (encounter_ids, "encounters"), (table_ids, "tables"), (investigation_ids, "investigations"), (abilities, "abilities"), (effects, "effects")):
        _unique(values, where=where)
    location_set, item_set, encounter_set, table_set = set(location_ids), set(item_ids), set(encounter_ids), set(table_ids)
    if "haven" not in location_set:
        raise ContentValidationError("configured campaign starting location is absent")
    for location in locations:
        if any(edge.destination_id not in location_set for edge in location.edges):
            raise ContentValidationError("travel edge references an unknown location")
        if location.investigation_id is not None and location.investigation_id not in investigation_ids:
            raise ContentValidationError("location references an unknown investigation")
    for investigation in investigations:
        if investigation.location_id not in location_set or (investigation.reward_table_id and investigation.reward_table_id not in table_set):
            raise ContentValidationError("investigation has an invalid reference")
    for threshold in thresholds:
        if threshold.item_id is not None and threshold.item_id not in item_set:
            raise ContentValidationError("progression threshold references an unknown item")
        if threshold.ability_id is not None and threshold.ability_id not in abilities:
            raise ContentValidationError("progression threshold references an unknown ability")
    for table in tables:
        for outcome in table.outcomes:
            if outcome.item_id is not None and outcome.item_id not in item_set:
                raise ContentValidationError("random outcome references an unknown item")
            if outcome.encounter_id is not None and outcome.encounter_id not in encounter_set:
                raise ContentValidationError("random outcome references an unknown encounter")
    for encounter in encounters:
        references = {encounter.player_hit_table_id, encounter.player_damage_table_id, encounter.enemy_hit_table_id, encounter.enemy_damage_table_id, encounter.escape_table_id, encounter.reward_table_id}
        if not references.issubset(table_set) or encounter.defeat_location_id not in location_set:
            raise ContentValidationError("encounter has an invalid table or location reference")
    final_location = _id(finale["location_id"], where="finale.location_id")
    final_table = _id(finale["table_id"], where="finale.table_id")
    final_gate = _gate(finale["gate"], where="finale.gate")
    if final_location not in location_set or final_table not in table_set:
        raise ContentValidationError("finale has an invalid reference")
    reachable = {"haven"}
    changed = True
    while changed:
        changed = False
        for location in locations:
            if location.location_id in reachable:
                for edge in location.edges:
                    if edge.destination_id not in reachable:
                        reachable.add(edge.destination_id)
                        changed = True
    if reachable != location_set:
        raise ContentValidationError("campaign contains an unreachable location")
    known_flags = {
        *(item.grants_flag for item in investigations),
        *(item.grants_flag for item in day_events),
        *(outcome.grants_flag for table in tables for outcome in table.outcomes if outcome.grants_flag),
    }
    gate_flags = {
        *(flag for location in locations for edge in location.edges for flag in edge.gate.required_flags),
        *final_gate.required_flags,
        *(flag for implication in implications for flag in implication.required_flags),
        *(implication.consequence_flag for implication in implications),
    }
    if not gate_flags.issubset(known_flags):
        raise ContentValidationError("quest gate references an unknown or unsatisfiable flag")

    return CampaignContent(
        version=version,
        locations=tuple(locations), investigations=tuple(investigations), random_tables=tuple(tables),
        final_location_id=final_location, final_gate=final_gate, final_table_id=final_table,
        known_items=frozenset(item_set), known_abilities=frozenset(abilities),
        known_encounters=frozenset(encounter_set), known_effects=frozenset(effects),
        available_profiles=frozenset({STANDARD_PROFILE}), equipment_slots=frozenset(slots),
        known_quest_flags=frozenset(known_flags), items=tuple(items), encounters=tuple(encounters),
        progression_thresholds=tuple(thresholds), day_events=tuple(day_events),
        quest_implications=tuple(implications), text_entries=tuple(sorted(text.items())),
    )


def load_standard_campaign(content_root: Path | None = None) -> CampaignContent:
    """Load and fully validate the sole MVP Effective Content Profile."""
    root = content_root or Path(__file__).resolve().parent
    standard_root = root / STANDARD_PROFILE
    manifest, hashes, _ = _load_manifest(standard_root)
    shipped = {"standard/manifest.toml", *(f"standard/{name}" for name in hashes)}
    _load_provenance(root, shipped)
    documents = {name: _read_json(standard_root / name, digest) for name, digest in hashes.items()}
    return _parse_campaign(documents, str(manifest["content_version"]))


def load_standard_snapshot(content_root: Path | None = None):
    """Return the immutable application snapshot with version and manifest hash."""
    from ..application import ContentSnapshot

    root = content_root or Path(__file__).resolve().parent
    campaign = load_standard_campaign(root)
    _, _, manifest_hash = _load_manifest(root / STANDARD_PROFILE)
    return ContentSnapshot(
        version=campaign.version,
        profile=STANDARD_PROFILE,
        manifest_hash=manifest_hash,
        records=(("campaign", campaign),),
    )


__all__ = [
    "ContentValidationError", "EXPECTED_FILES", "STANDARD_PROFILE",
    "load_standard_campaign", "load_standard_snapshot",
]
