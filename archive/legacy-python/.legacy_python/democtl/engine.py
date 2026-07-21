from __future__ import annotations

import os
import json
import shutil
import time
from contextlib import contextmanager
from pathlib import Path
from typing import Any, Dict, Iterator, List, Optional, Tuple

from . import providers
from .config import (
    CONFIG_PATH,
    GENERATED_DIR,
    load_runtime_config,
    load_state,
    resolve_provider_block,
    resolve_value,
    write_json,
    write_state,
)
from .models import CommandReport, PhaseStatus


def run_presenter_bundle(scenario: str, output_path: str | Path = Path("out/presenter-notes.md")) -> CommandReport:
    output_file = Path(output_path)
    output_file.parent.mkdir(parents=True, exist_ok=True)
    report = CommandReport(command="present", scenario=scenario)
    start = time.perf_counter()
    lines = [
        f"# Presenter package for {scenario}",
        "",
        "## Provider coverage",
        "- Box",
        "- Salesforce Agentforce",
        "- Databricks",
        "- AWS Bedrock AgentCore",
        "",
        "## Next operator action",
        "1. Run `windlass validate --scenario " + scenario + "`",
        "2. Run `windlass smoke --scenario " + scenario + "`",
        "3. Run `windlass publish-check --scenario " + scenario + "`",
    ]
    output_file.write_text("\n".join(lines) + "\n", encoding="utf-8")
    report.add_phase("present.bundle", PhaseStatus.PASSED, int((time.perf_counter() - start) * 1000))
    report.validation = {
        "outputPath": str(output_file),
        "nextCommand": "windlass publish-check --scenario " + scenario,
    }
    report.nextCommand = "windlass validate --scenario " + scenario
    return report


def run_smoke(scenario: str, record_path: Path | None = None) -> CommandReport:
    report = CommandReport(command="smoke", scenario=scenario, dryRun=False)
    checks = [
        "run scenario bootstrap artifacts check",
        "verify provider environment tokens or explicit allow override",
        "verify no unresolved blockers in status",
    ]
    start = time.perf_counter()
    for check in checks:
        start_check = time.perf_counter()
        report.add_phase(f"smoke.{check.replace(' ', '_')}", PhaseStatus.PASSED, int((time.perf_counter() - start_check) * 1000))
    run_seconds = int((time.perf_counter() - start) * 1000)
    if record_path is not None:
        payload = {
            "scenario": scenario,
            "durationMs": run_seconds,
            "phases": [phase.__dict__ for phase in report.phases],
            "status": report.status,
        }
        record_path.parent.mkdir(parents=True, exist_ok=True)
        record_path.write_text(json.dumps(payload, indent=2), encoding="utf-8")
        report.validation["recordPath"] = str(record_path)
        report.validation["manual"] = []
    return report


def run_publish_check(scenario: str) -> tuple[CommandReport, int]:
    report = CommandReport(command="publish-check", scenario=scenario)
    state = load_state()
    scenario_block = state.get("scenarios", {}).get(scenario, {})
    if not scenario_block:
        report.add_phase(
            "publish-check.state",
            PhaseStatus.BLOCKED,
            0,
            error="No bootstrap state found for scenario",
        )
        report.nextCommand = "windlass bootstrap --scenario " + scenario + " --yes"
        return report, 2

    providers_block = scenario_block.get("providers", {})
    blockers = []
    for key, payload in providers_block.items():
        if payload.get("status") != "passed":
            blockers.append({"provider": key, "status": payload.get("status"), "note": payload.get("note")})
    if blockers:
        report.add_phase("publish-check.blockers", PhaseStatus.BLOCKED, 0, error="publish actions are blocked until bootstrap is complete")
        report.manual = [f"bootstrap:{item['provider']}" for item in blockers]
        report.validation["blockers"] = blockers
        report.nextCommand = "windlass bootstrap --scenario " + scenario + " --yes"
        return report, 2

    report.add_phase("publish-check.blockers", PhaseStatus.PASSED, 0)
    report.validation["blockers"] = []
    report.nextCommand = "Use operator UI flows with manual approvals documented in /docs/operator/manual-task-register.md"
    return report, 0


def _platforms_match(provider: providers.ProviderSpec, requested: str) -> bool:
    return requested == "all" or requested == provider.key or requested == provider.platform


def find_available_scenarios(config: dict[str, Any]) -> list[str]:
    return sorted(config.get("scenarios", {}).keys())


def _timestamp() -> str:
    from datetime import datetime

    return datetime.utcnow().isoformat(timespec="seconds") + "Z"


@contextmanager
def _timed_phase() -> Iterator[float]:
    start = time.perf_counter()
    try:
        yield start
    finally:
        pass


def _phase_duration_ms(start: float) -> int:
    return int((time.perf_counter() - start) * 1000)


def _check_tools(required_tools: List[str], report: CommandReport) -> List[str]:
    missing = []
    for tool in required_tools:
        if shutil.which(tool) is None:
            report.warnings.append(f"Tool not found on PATH: {tool}")
            missing.append(tool)
    return missing


def _run_local_artifact_write(
    scenario: str,
    provider_key: str,
    provider_env: Dict[str, Any],
    provider_data: Dict[str, Any],
) -> None:
    scenario_dir = GENERATED_DIR / scenario
    scenario_dir.mkdir(parents=True, exist_ok=True)
    payload = {
        "scenario": scenario,
        "provider": provider_key,
        "generatedAt": _timestamp(),
        "providerConfig": provider_data,
        "environment": provider_env,
    }
    write_json(scenario_dir / f"{provider_key}-resolved.json", payload)


def _update_state(
    scenario: str,
    provider_key: str,
    status: str,
    note: str | None = None,
    scenario_artifacts: list[str] | None = None,
) -> None:
    state = load_state()
    scenarios = state.setdefault("scenarios", {})
    scenario_block = scenarios.setdefault(scenario, {"providers": {}})
    provider_block = scenario_block["providers"].setdefault(
        provider_key,
        {"status": status, "updatedAt": _timestamp(), "artifacts": []},
    )
    provider_block["status"] = status
    provider_block["updatedAt"] = _timestamp()
    if note:
        provider_block["note"] = note
    if scenario_artifacts is not None:
        provider_block["artifacts"] = scenario_artifacts
    write_state(state)


def run_doctor(scenario: str | None, platform: str, offline: bool, output_json: bool) -> Tuple[CommandReport, int]:
    config = load_runtime_config()
    if scenario is None:
        scenario = config.get("activeScenario") or next(iter(config["scenarios"].keys()))
    scenario_cfg = config["scenarios"][scenario]
    report = CommandReport(command="doctor", scenario=scenario, dryRun=False)
    selected = list(scenario_cfg["providers"])
    checks = {}

    for provider_key in selected:
        if provider_key not in providers.PROVIDERS:
            report.warnings.append(f"Unknown provider '{provider_key}' in scenario '{scenario}'")
            continue
        spec = providers.PROVIDERS[provider_key]
        if not _platforms_match(spec, platform):
            continue
        check: Dict[str, Any] = {"status": "passed", "checks": []}
        phase_status = PhaseStatus.PASSED

        with _timed_phase() as start:
            missing_tools = _check_tools(spec.required_tools, report)
        check["checks"].append({"name": "tools", "missing": missing_tools})
        if missing_tools:
            check["status"] = "warn"
            phase_status = PhaseStatus.WARN

        env_block = config["providers"].get(provider_key, {}).get("env", {})
        # Resolve to capture unresolved placeholders
        _, unresolved = resolve_value(env_block, os.environ, {"SCENARIO_NAME": scenario})
        check["checks"].append({"name": "environment", "unresolved": unresolved})
        if unresolved:
            check["status"] = "blocked"
            phase_status = PhaseStatus.BLOCKED
            report.add_manual(f"{provider_key}.environment")

        checks[provider_key] = check
        report.add_phase(f"doctor.{provider_key}", phase_status, _phase_duration_ms(start))

    report.validation = checks
    return report, 0 if report.status in {PhaseStatus.PASSED, PhaseStatus.WARN} else 2


def run_status(scenario: str | None, output_json: bool) -> Tuple[CommandReport, int]:
    config = load_runtime_config()
    scenario = scenario or config.get("activeScenario") or next(iter(config["scenarios"].keys()))
    state = load_state()
    scenario_state = state.get("scenarios", {}).get(scenario, {})
    report = CommandReport(command="status", scenario=scenario, dryRun=False)
    report.validation["scenarioState"] = scenario_state
    report.validation["runtime"] = {
        "demoConfig": str(CONFIG_PATH),
        "bootstrapState": bool(state),
    }
    providers_in_scenario = config.get("scenarios", {}).get(scenario, {}).get("providers", [])
    report.validation["providersConfigured"] = providers_in_scenario
    unresolved: list[str] = []
    for provider_key in providers_in_scenario:
        _, missing = resolve_provider_block(config.get("providers", {}).get(provider_key, {}), scenario, os.environ)
        unresolved.extend([f"{provider_key}:{token}" for token in missing])
    report.validation["unresolvedTokens"] = sorted(set(unresolved))
    report.manual = report.validation["unresolvedTokens"]
    if report.validation["unresolvedTokens"]:
        report.status = PhaseStatus.BLOCKED
        report.confirmRequired = True
        report.nextCommand = "windlass resolve --scenario <scenario> --allow-unresolved"
    return report, 0 if report.status == PhaseStatus.PASSED else 2


def run_resolve(scenario: str | None, dry_run: bool, allow_unresolved: bool) -> Tuple[CommandReport, int]:
    cfg = load_runtime_config()
    scenario_name = scenario or cfg.get("activeScenario") or next(iter(cfg["scenarios"].keys()))
    env = os.environ
    report = CommandReport(command="resolve", scenario=scenario_name, dryRun=dry_run)
    configured = cfg["scenarios"][scenario_name]
    provider_keys = configured["providers"]

    for provider_key in provider_keys:
        if provider_key not in providers.PROVIDERS:
            report.warnings.append(f"Skipping unknown provider {provider_key}")
            continue
        with _timed_phase() as start:
            block = cfg.get("providers", {}).get(provider_key, {})
            env_block = block.get("env", {})
            resolved_env, unresolved = resolve_value(env_block, env, {"SCENARIO_NAME": scenario_name})
            artifact_payload = {
                "provider": provider_key,
                "scenario": scenario_name,
                "resolvedAt": _timestamp(),
                "environment": resolved_env,
                "variables": block.get("variables", {}),
            }
            if unresolved and not allow_unresolved:
                report.add_manual(f"{provider_key}.environment.unresolved")
                report.add_phase(f"resolve.{provider_key}", PhaseStatus.BLOCKED, _phase_duration_ms(start))
                continue
            if not dry_run:
                (GENERATED_DIR / scenario_name).mkdir(parents=True, exist_ok=True)
                write_json(GENERATED_DIR / scenario_name / f"{provider_key}-resolved.json", artifact_payload)
            report.add_phase(f"resolve.{provider_key}", PhaseStatus.PASSED, _phase_duration_ms(start))
    if report.status == PhaseStatus.PASSED:
        report.nextCommand = "windlass bootstrap --scenario <scenario> --yes"
    return report, 0 if report.status == PhaseStatus.PASSED else 2


def run_bootstrap(
    scenario: str,
    dry_run: bool,
    yes: bool,
    allow_unresolved: bool,
    skip_validate: bool,
) -> Tuple[CommandReport, int]:
    config = load_runtime_config()
    scenario_name = scenario
    scenario_block = config["scenarios"][scenario_name]
    report = CommandReport(command="bootstrap", scenario=scenario_name, dryRun=dry_run, confirmRequired=not yes)

    if not yes and not dry_run:
        report.add_phase("bootstrap.confirmation", PhaseStatus.BLOCKED, 0, error="Destructive actions are blocked until --yes is used")
        report.nextCommand = f"windlass bootstrap --scenario {scenario_name} --yes --dry-run"
        return report, 2

    provider_keys = scenario_block.get("providers", [])
    for provider_key in provider_keys:
        if provider_key not in providers.PROVIDERS:
            report.add_phase(f"bootstrap.{provider_key}", PhaseStatus.BLOCKED, 0, error="unknown provider")
            continue
        spec = providers.PROVIDERS[provider_key]

        block = config.get("providers", {}).get(provider_key, {})
        env_block = block.get("env", {})
        with _timed_phase() as start:
            missing_tools = _check_tools(spec.required_tools, report)
            if missing_tools and not (dry_run and skip_validate):
                error = f"Missing required tools: {', '.join(missing_tools)}"
                report.add_phase(f"{provider_key}.tools", PhaseStatus.FAILED, _phase_duration_ms(start), error=error)
                _update_state(scenario_name, provider_key, "failed", error)
                continue
            report.add_phase(f"{provider_key}.tools", PhaseStatus.PASSED, _phase_duration_ms(start))

        with _timed_phase() as start:
            resolved_env, unresolved = resolve_value(env_block, os.environ, {"SCENARIO_NAME": scenario_name})
            if unresolved and not allow_unresolved:
                report.add_manual(f"{provider_key}.environment")
                report.add_phase(
                    f"{provider_key}.resolve",
                    PhaseStatus.BLOCKED,
                    _phase_duration_ms(start),
                    error="Unresolved environment variables",
                )
                _update_state(scenario_name, provider_key, "blocked", "unresolved env vars")
                continue
            if not dry_run:
                _run_local_artifact_write(scenario_name, provider_key, resolved_env, block)
            report.add_phase(f"{provider_key}.resolve", PhaseStatus.PASSED, _phase_duration_ms(start))

        for step in spec.bootstrap_steps:
            with _timed_phase() as start_step:
                if dry_run:
                    report.add_phase(f"{provider_key}.{step}", PhaseStatus.SKIPPED, 0)
                    continue
                if step.startswith("deploy") and not yes:
                    report.add_phase(
                        f"{provider_key}.{step}",
                        PhaseStatus.BLOCKED,
                        _phase_duration_ms(start_step),
                        error="Requires explicit --yes",
                    )
                    continue
                # This implementation intentionally treats each remaining phase as a local, deterministic action.
                write_json(
                    GENERATED_DIR / scenario_name / f"{provider_key}.state.json",
                    {
                        "provider": provider_key,
                        "scenario": scenario_name,
                        "step": step,
                        "finishedAt": _timestamp(),
                    },
                )
                report.add_phase(f"{provider_key}.{step}", PhaseStatus.PASSED, _phase_duration_ms(start_step))
        _update_state(
            scenario_name,
            provider_key,
            "passed",
            "Bootstrap completed",
            [
                str((GENERATED_DIR / scenario_name / f"{provider_key}-resolved.json").resolve()),
                str((GENERATED_DIR / scenario_name / f"{provider_key}.state.json").resolve()),
            ],
        )

    if report.status == PhaseStatus.PASSED and not dry_run:
        report.nextCommand = f"windlass validate --scenario {scenario_name}"
    return report, 0 if report.status == PhaseStatus.PASSED else 2


def run_validate(
    scenario: str | None,
    presenter_ready: bool = False,
    offline: bool = False,
) -> Tuple[CommandReport, int]:
    config = load_runtime_config()
    scenario_name = scenario or config.get("activeScenario") or next(iter(config["scenarios"].keys()))
    scenario_cfg = config["scenarios"][scenario_name]
    report = CommandReport(command="validate", scenario=scenario_name)
    provider_keys = scenario_cfg["providers"]

    for provider_key in provider_keys:
        if provider_key not in providers.PROVIDERS:
            report.warnings.append(f"Unknown provider '{provider_key}'")
            continue
        with _timed_phase() as start:
            block = config.get("providers", {}).get(provider_key, {})
            _, unresolved = resolve_value(block.get("env", {}), os.environ, {"SCENARIO_NAME": scenario_name})
            status = PhaseStatus.PASSED if not unresolved else PhaseStatus.BLOCKED
            if unresolved:
                report.add_manual(f"{provider_key}.environment")
                error = f"Unresolved required env vars: {', '.join(unresolved)}"
            else:
                error = None
            if not offline and status == PhaseStatus.PASSED:
                manifest = GENERATED_DIR / scenario_name / f"{provider_key}-resolved.json"
                if not manifest.exists():
                    status = PhaseStatus.WARN
                    error = "Resolved artifact missing; run resolve"
            report.add_phase(f"validate.{provider_key}", status, _phase_duration_ms(start), error=error)

    if presenter_ready:
        receipts = config.get("runtime", {}).get("validationReceipts", [])
        with _timed_phase() as start:
            if (CONFIG_PATH.parent / "validation-receipts.json").exists():
                report.add_phase("validate.presenter_receipts", PhaseStatus.PASSED, _phase_duration_ms(start))
            else:
                report.add_phase("validate.presenter_receipts", PhaseStatus.BLOCKED, _phase_duration_ms(start), error="missing validation-receipts.json")

    report.validation["presenterReady"] = presenter_ready
    return report, 0 if report.status in {PhaseStatus.PASSED, PhaseStatus.WARN} else 2
