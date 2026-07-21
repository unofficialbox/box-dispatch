from __future__ import annotations

import json
import re
from copy import deepcopy
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, Iterable, List, Tuple

from .models import CommandReport

ROOT = Path(__file__).resolve().parents[1]
CONFIG_DIR = ROOT / "config" / "runtime"
EXAMPLE_PATH = CONFIG_DIR / "demo-environment.example.json"
CONFIG_PATH = CONFIG_DIR / "demo-environment.json"
STATE_PATH = CONFIG_DIR / "bootstrap-state.json"
GENERATED_DIR = CONFIG_DIR / "generated"
VALIDATION_RECEIPTS_PATH = CONFIG_DIR / "validation-receipts.json"

TOKEN_RE = re.compile(r"\$\{([A-Z0-9_]+)\}")


def _now() -> str:
    return datetime.utcnow().isoformat(timespec="seconds") + "Z"


def read_json(path: Path) -> dict[str, Any]:
    with path.open("r", encoding="utf-8") as handle:
        return json.load(handle)


def write_json(path: Path, payload: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("w", encoding="utf-8") as handle:
        json.dump(payload, handle, indent=2)
        handle.write("\n")


def load_runtime_config() -> dict[str, Any]:
    if not CONFIG_PATH.exists():
        raise FileNotFoundError(
            f"{CONFIG_PATH} is missing. Run `windlass setup` to create it from "
            f"{EXAMPLE_PATH}."
        )
    return read_json(CONFIG_PATH)


def load_state() -> dict[str, Any]:
    if not STATE_PATH.exists():
        return {"generatedAt": _now(), "scenarios": {}}
    return read_json(STATE_PATH)


def write_state(state: dict[str, Any]) -> None:
    state["generatedAt"] = _now()
    write_json(STATE_PATH, state)


def resolve_value(value: Any, env: Dict[str, str], context: Dict[str, str]) -> Tuple[Any, List[str]]:
    unresolved: List[str] = []
    if isinstance(value, str):
        def repl(match: re.Match[str]) -> str:
            token = match.group(1)
            if token in context:
                return context[token]
            if token in env:
                return env[token]
            unresolved.append(token)
            return match.group(0)

        return TOKEN_RE.sub(repl, value), unresolved
    if isinstance(value, list):
        out_list = []
        for item in value:
            resolved_item, unresolved_item = resolve_value(item, env, context)
            out_list.append(resolved_item)
            unresolved.extend(unresolved_item)
        return out_list, unresolved
    if isinstance(value, dict):
        out_map = {}
        for key, item in value.items():
            resolved_item, unresolved_item = resolve_value(item, env, context)
            out_map[key] = resolved_item
            unresolved.extend(unresolved_item)
        return out_map, unresolved
    return value, unresolved


def resolve_provider_block(config_block: dict[str, Any], scenario: str, env: Dict[str, str]) -> Tuple[dict[str, Any], List[str]]:
    context = {"SCENARIO_NAME": scenario}
    payload = deepcopy(config_block)
    resolved, unresolved = resolve_value(payload, env, context)
    return resolved, sorted(set(unresolved))


def scenario_or_default(config: dict[str, Any], scenario: str | None) -> str:
    if scenario:
        if scenario not in config.get("scenarios", {}):
            raise ValueError(f"Scenario {scenario} is not defined.")
        return scenario
    active = config.get("activeScenario")
    if active and active in config.get("scenarios", {}):
        return active
    available = list(config.get("scenarios", {}).keys())
    if not available:
        raise ValueError("No scenarios are configured.")
    return available[0]


def initialize_runtime(force: bool = False) -> dict[str, Any]:
    if not EXAMPLE_PATH.exists():
        raise FileNotFoundError(f"{EXAMPLE_PATH} is required to bootstrap.")
    if CONFIG_PATH.exists() and not force:
        return load_runtime_config()

    payload = read_json(EXAMPLE_PATH)
    write_json(CONFIG_PATH, payload)
    initial_state = {"generatedAt": _now(), "scenarios": {}}
    write_state(initial_state)
    return payload


def required_env_keys(config_path: Path | None = None) -> list[str]:
    try:
        cfg = load_runtime_config() if config_path is None else read_json(config_path)
    except Exception:
        return []
    keys: set[str] = set()
    providers_cfg = cfg.get("providers", {})
    for provider_block in providers_cfg.values():
        keys.update(provider_block.get("env", {}).values())
    expanded: set[str] = set()
    for token_expr in keys:
        if isinstance(token_expr, str):
            expanded.update(TOKEN_RE.findall(token_expr))
    return sorted(expanded)
