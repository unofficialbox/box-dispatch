from __future__ import annotations

import os
from pathlib import Path
from typing import Optional

import typer

from . import config
from .engine import (
    find_available_scenarios,
    run_bootstrap,
    run_doctor,
    run_presenter_bundle,
    run_publish_check,
    run_resolve,
    run_smoke,
    run_status,
    run_validate,
)
from .models import CommandReport
from .reporter import print_report


app = typer.Typer(help="Demo scenario CLI for Box, Salesforce Agentforce, Databricks, and Bedrock AgentCore")


def _resolve_scenario(cfg: dict, scenario: Optional[str]) -> str:
    try:
        return config.scenario_or_default(cfg, scenario)
    except ValueError as error:
        raise typer.BadParameter(str(error))


@app.command()
def setup(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario key to validate after setup"),
    force: bool = typer.Option(False, "--force", help="Overwrite existing runtime config"),
    dry_run: bool = typer.Option(False, "--dry-run", "-n", help="Show planned write operations"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    report = CommandReport(command="setup", status="passed", dryRun=dry_run)
    if dry_run:
        report.add_phase("setup.init", "skipped", 0)
        print_report(report, as_json=json)
        raise typer.Exit()

    cfg = config.initialize_runtime(force=force)
    if scenario is not None:
        if scenario not in cfg.get("scenarios", {}):
            raise typer.BadParameter(f"Scenario {scenario} is not in {config.EXAMPLE_PATH}")
        cfg["activeScenario"] = scenario
        config.write_json(config.CONFIG_PATH, cfg)

    report = CommandReport(command="setup", scenario=cfg.get("activeScenario"), dryRun=False)
    report.add_phase("setup.init", "passed", 0)
    report.validation = {"state": str(config.STATE_PATH), "config": str(config.CONFIG_PATH)}
    report.nextCommand = "windlass doctor --scenario <scenario>"
    print_report(report, as_json=json)


@app.command()
def doctor(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to check"),
    platform: str = typer.Option("all", "--platform", help="Filter checks by provider key or platform"),
    offline: bool = typer.Option(False, "--offline", help="Skip availability checks that are network-backed"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report, code = run_doctor(scenario_name, platform, offline, json)
    report.scenario = scenario_name
    print_report(report, as_json=json)
    raise typer.Exit(code=code)


@app.command()
def bootstrap(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to bootstrap"),
    dry_run: bool = typer.Option(False, "--dry-run", "-n", help="Plan only; do not write artifacts"),
    yes: bool = typer.Option(False, "--yes", "--confirm", help="Allow destructive steps"),
    allow_unresolved: bool = typer.Option(False, "--allow-unresolved", help="Allow unresolved environment values"),
    skip_validate: bool = typer.Option(False, "--skip-validate", help="Skip remote availability checks"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report, code = run_bootstrap(scenario_name, dry_run, yes, allow_unresolved, skip_validate)
    report.scenario = scenario_name
    print_report(report, as_json=json)
    raise typer.Exit(code=code)


@app.command()
def status(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to inspect"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report, code = run_status(scenario_name, json)
    report.scenario = scenario_name
    print_report(report, as_json=json)
    raise typer.Exit(code=code)


@app.command()
def resolve(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to resolve"),
    dry_run: bool = typer.Option(False, "--dry-run", "-n", help="Plan only"),
    allow_unresolved: bool = typer.Option(False, "--allow-unresolved", help="Allow unresolved placeholders"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report, code = run_resolve(scenario_name, dry_run, allow_unresolved)
    report.scenario = scenario_name
    print_report(report, as_json=json)
    raise typer.Exit(code=code)


@app.command()
def validate(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to validate"),
    presenter_ready: bool = typer.Option(False, "--presenter-ready", help="Require validation-receipts.json"),
    offline: bool = typer.Option(False, "--offline", help="Skip artifact and integration expectations"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report, code = run_validate(scenario_name, presenter_ready=presenter_ready, offline=offline)
    report.scenario = scenario_name
    print_report(report, as_json=json)
    raise typer.Exit(code=code)


@app.command()
def present(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to materialize presenter package for"),
    output: str = typer.Option("out/presenter-notes.md", "--output", help="Output markdown path"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report = run_presenter_bundle(scenario_name, output_path=Path(output))
    print_report(report, as_json=json)
    raise typer.Exit(code=0)


@app.command()
def smoke(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to smoke test"),
    record_path: Optional[str] = typer.Option(None, "--record-path", help="Path for JSON smoke run output"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report = run_smoke(scenario_name, record_path=Path(record_path) if record_path else None)
    print_report(report, as_json=json)
    raise typer.Exit(code=0 if report.status == "passed" else 2)


@app.command()
def publish_check(
    scenario: Optional[str] = typer.Option(None, "--scenario", help="Scenario to evaluate publish safety"),
    json: bool = typer.Option(False, "--json", help="Emit machine-readable output"),
) -> None:
    cfg = config.load_runtime_config()
    scenario_name = _resolve_scenario(cfg, scenario)
    report, code = run_publish_check(scenario_name)
    print_report(report, as_json=json)
    raise typer.Exit(code=code)


@app.command()
def scenarios() -> None:
    cfg = config.load_runtime_config()
    names = find_available_scenarios(cfg)
    print("Available scenarios:")
    for name in names:
        if name == cfg.get("activeScenario"):
            print(f"  * {name} (active)")
        else:
            print(f"    {name}")


@app.command()
def env() -> None:
    print("Environment variables read by this CLI:")
    for key in sorted(config.required_env_keys()):
        state = "set" if key in os.environ else "missing"
        print(f"  - {key}: {state}")


def main() -> None:
    app()


if __name__ == "__main__":
    main()
