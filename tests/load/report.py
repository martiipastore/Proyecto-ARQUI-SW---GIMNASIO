"""D13: resumir evidencia conservada, sin ejecutar carga ni consultar servicios."""

import argparse
import json
from pathlib import Path


def render(evidence: Path, probe: bool = False) -> str:
    lines = [
        "# Resultados de carga de Benefits",
        "",
        "Generado desde los JSON de cada fase; los percentiles no se promedian entre fases.",
        "Tiempos HTTP de k6 en ms; throughput en solicitudes/s. Calentamiento excluido de esta tabla.",
        "",
        "| Escenario | VU | req/s | Operación | p50 ms | p95 ms | p99 ms | Umbral |",
        "|---|---:|---:|---|---:|---:|---:|---|",
    ]
    cases = ["write-hot"] if probe else [f"{w}-{d}" for w in ("read", "write", "replay", "mixed") for d in ("hot", "distributed")]
    missing, failures, latency_failures, resources, invariants, plans = [], [], [], {}, [], []
    for case in cases:
        final = None
        for phase in ("warmup", "vu1", "vu5", "vu10"):
            path = evidence / case / (phase + ".json")
            if not path.exists():
                missing.append(f"{case}/{phase}")
                continue
            data = json.loads(path.read_text(encoding="utf-8-sig"))
            summary, metrics = data["summary"], data["summary"]["metrics"]
            final = data
            functional = metrics["functional_failures"]["values"]["rate"]
            if functional != 0 or not data["invariants_ok"]:
                failures.append(f"{case}/{phase}")
            for sample in data["samples"]:
                for service in sample["services"]:
                    peaks = resources.setdefault(service["service"], [0, 0])
                    peaks[0] = max(peaks[0], service["cpu_percent"])
                    peaks[1] = max(peaks[1], service["memory_bytes"])
                for field in ("connections", "active", "lock_waiters", "io_waiters"):
                    name = "postgres_" + field
                    resources[name] = max(resources.get(name, 0), sample["postgres"][field])
                resources["other_cpu"] = max(resources.get("other_cpu", 0), sample["other_cpu_percent"])
                resources["other_memory"] = max(resources.get("other_memory", 0), sample["other_memory_bytes"])
            if phase == "warmup":
                continue
            for name in ("read_ms", "write_ms", "replay_ms"):
                metric = metrics.get(name)
                if metric is None or not metric.get("thresholds"):
                    continue
                values = metric["values"]
                passed = all(t["ok"] for t in metric["thresholds"].values())
                if not passed:
                    latency_failures.append(f"{case}/{phase}/{name}")
                lines.append(f"| {case} | {summary['vus']} | {metrics['http_reqs']['values']['rate']:.2f} | {name} | {values['p(50)']:.2f} | {values['p(95)']:.2f} | {values['p(99)']:.2f} | {'Cumple' if passed else 'No cumple'} |")
        if final:
            state = final["state"]
            invariants.append(f"| {case} | {state['accounts']} | {state['balance_sum']} | {state['movements']} | {state['operations']} | {state['inconsistent_accounts']} |")
        plan_path = evidence / case / "ledger-plan.json"
        if plan_path.exists():
            plan = json.loads(plan_path.read_text(encoding="utf-8-sig"))[0]
            scans = plan["Plan"].get("Plans", [])
            scan = scans[0] if scans else plan["Plan"]
            plans.append(f"| {case} | {scan['Node Type']} | {scan.get('Actual Rows', 0)} | {plan['Execution Time']:.3f} |")
    lines.extend(["", "## Estado final por escenario", "", "Incluye precarga y calentamiento. Las lecturas y los replays no deben crear movimientos.", "", "| Escenario | Cuentas | Suma saldos | Movimientos | Operaciones | Cuentas inconsistentes |", "|---|---:|---:|---:|---:|---:|"] + invariants)
    lines.extend(["", "## Máximos de las muestras de recursos", "", "Muestreo objetivo cada 5 s más el tiempo de consulta; no representa máximos instantáneos ni prueba ausencia de esperas breves.", "CPU Docker: 100 % equivale aproximadamente a un núcleo; valores mayores son posibles.", "", "| Servicio | CPU % | Memoria MiB |", "|---|---:|---:|"])
    for name, values in sorted(resources.items()):
        if isinstance(values, list):
            lines.append(f"| {name} | {values[0]:.2f} | {values[1] / 1048576:.2f} |")
    if resources:
        lines.extend([f"| Otros contenedores, agregados | {resources['other_cpu']:.2f} | {resources['other_memory'] / 1048576:.2f} |", "", f"PostgreSQL: hasta {resources['postgres_connections']} sesiones en benefits_db (incluye procesos internos y otras conexiones; excluye la consulta de monitoreo); {resources['postgres_active']} activas; {resources['postgres_lock_waiters']} esperando locks y {resources['postgres_io_waiters']} esperando I/O en las muestras (máximos independientes)."])
    if plans:
        lines.extend(["", "## Plan del agregado del ledger, cuenta 0", "", "Consulta diagnóstica posterior a cada combinación; no mide la transacción completa ni modifica el esquema.", "", "| Escenario | Acceso elegido | Filas devueltas al agregado | Ejecución ms |", "|---|---|---:|---:|"] + plans)
    lines.extend(["", f"Fases pendientes: {len(missing)}. Fases con falla funcional o inconsistencia: {len(failures)}."])
    lines.append(f"Objetivos de latencia incumplidos en niveles medidos (sin calentamiento): {len(latency_failures)}.")
    if missing:
        lines.extend(["", "Reporte parcial: " + ", ".join(missing) + "."])
    if failures:
        lines.extend(["", "Revisar: " + ", ".join(failures) + "."])
    if latency_failures:
        lines.extend(["", "Latencia fuera del objetivo: " + ", ".join(latency_failures) + "."])
    return "\n".join(lines) + "\n"


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("evidence", type=Path, nargs="?", default=Path("docs/evidence/etapa7"))
    parser.add_argument("--probe", action="store_true", help="Solo experimento adicional write-hot, D-59")
    args = parser.parse_args()
    (args.evidence / "RESULTADOS.md").write_text(render(args.evidence, args.probe), encoding="utf-8")
