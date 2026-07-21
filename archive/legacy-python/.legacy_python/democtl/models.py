from __future__ import annotations

from dataclasses import dataclass, field, asdict
from enum import Enum
from typing import Any, Dict, List, Optional


class PhaseStatus(str, Enum):
    PASSED = "passed"
    SKIPPED = "skipped"
    BLOCKED = "blocked"
    FAILED = "failed"
    WARN = "warn"


@dataclass
class PhaseResult:
    name: str
    status: str
    duration_ms: int
    error: Optional[str] = None


@dataclass
class CommandReport:
    command: str
    scenario: Optional[str] = None
    dryRun: bool = False
    confirmRequired: bool = False
    phases: List[PhaseResult] = field(default_factory=list)
    manual: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)
    validation: Dict[str, Any] = field(default_factory=dict)
    status: str = PhaseStatus.PASSED
    nextCommand: Optional[str] = None

    def add_phase(self, name: str, status: str, duration_ms: int, error: Optional[str] = None) -> None:
        self.phases.append(PhaseResult(name=name, status=status, duration_ms=duration_ms, error=error))
        if status == PhaseStatus.BLOCKED:
            self.status = PhaseStatus.BLOCKED
            self.confirmRequired = True
        elif status == PhaseStatus.FAILED and self.status != PhaseStatus.BLOCKED:
            self.status = PhaseStatus.FAILED

    def add_manual(self, item: str) -> None:
        if item not in self.manual:
            self.manual.append(item)

    def to_dict(self) -> Dict[str, Any]:
        payload = asdict(self)
        payload["status"] = str(self.status)
        for phase in payload["phases"]:
            phase["status"] = str(phase["status"])
        return payload

