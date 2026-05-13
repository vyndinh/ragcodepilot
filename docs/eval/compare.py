#!/usr/bin/env python3
"""Compare one or more ragcodepilot eval JSON reports.

Usage:
    docs/eval/compare.py REPORT [REPORT ...]
    docs/eval/compare.py --labels=baseline,dense,hybrid r1.json r2.json r3.json

Behavior:
    - One report:  print metadata + aggregate metrics table for that report.
    - Two+ reports: print metadata, side-by-side metrics table, and pairwise
      deltas (in percentage points) relative to the first report.

Labels default to each file's basename without `.json`. Override with
`--labels=a,b,c` if you want shorter or more descriptive column headers.

Stdlib only — no pip install required.

Examples:
    # Summarize a single baseline:
    docs/eval/compare.py docs/eval/baseline_v2.json

    # Phase 2 sweep (mirrors the table in docs/plan/hybrid_search.md):
    docs/eval/compare.py \\
        docs/eval/baseline_v1.json \\
        /tmp/eval_dense.json \\
        /tmp/eval_sparse.json \\
        /tmp/eval_hybrid.json \\
        --labels=baseline_v1,dense_p2,sparse_p2,hybrid_p2

    # Regression check on a candidate run:
    docs/eval/compare.py docs/eval/baseline_v2.json /tmp/candidate.json
"""

import argparse
import json
import sys
from pathlib import Path


HEADERS = [
    "Run",
    "h@1", "h@3", "h@5",
    "MRR@5", "rcl@10",
    "nav h@5", "con h@5", "beh h@5",
    "neg pass",
    "p50/p95 ms",
]


def load_report(path: Path) -> dict:
    with path.open() as f:
        return json.load(f)


def fmt_num(x) -> str:
    if isinstance(x, float):
        return f"{x:.3f}"
    if x is None:
        return "-"
    return str(x)


def aggregate_row(label: str, report: dict) -> list[str]:
    agg = report.get("aggregate", {})
    by_type = report.get("by_type", {})
    nav = by_type.get("navigation", {})
    con = by_type.get("concept", {})
    beh = by_type.get("behavior", {})
    neg = by_type.get("negative", {})
    return [
        label,
        fmt_num(agg.get("hit_at_1", 0)),
        fmt_num(agg.get("hit_at_3", 0)),
        fmt_num(agg.get("hit_at_5", 0)),
        fmt_num(agg.get("mrr_at_5", 0)),
        fmt_num(agg.get("recall_at_10", 0)),
        fmt_num(nav.get("hit_at_5", 0)),
        fmt_num(con.get("hit_at_5", 0)),
        fmt_num(beh.get("hit_at_5", 0)),
        fmt_num(neg.get("negative_pass_rate", 0)),
        f"{agg.get('latency_total_p50_ms', 0)}/{agg.get('latency_total_p95_ms', 0)}",
    ]


def print_metadata(reports: list[dict], labels: list[str]) -> None:
    print("Reports:")
    for label, report in zip(labels, reports):
        run_id = report.get("run_id", "-")
        embedder = report.get("embedder", "-")
        mode = report.get("mode", "-")
        dataset = report.get("dataset", "-")
        print(f"  {label}: run={run_id}  mode={mode}  embedder={embedder}  dataset={dataset}")
    print()


def print_table(rows: list[list[str]]) -> None:
    widths = [max(len(h), max(len(r[i]) for r in rows)) for i, h in enumerate(HEADERS)]

    def line(cells: list[str]) -> str:
        return "  ".join(c.ljust(w) for c, w in zip(cells, widths))

    print(line(HEADERS))
    print(line(["-" * w for w in widths]))
    for r in rows:
        print(line(r))


# Metric extractors used for delta computation. Each tuple is (label, getter).
DELTA_METRICS = [
    ("hit@1", lambda r: r["aggregate"].get("hit_at_1", 0)),
    ("hit@3", lambda r: r["aggregate"].get("hit_at_3", 0)),
    ("hit@5", lambda r: r["aggregate"].get("hit_at_5", 0)),
    ("MRR@5", lambda r: r["aggregate"].get("mrr_at_5", 0)),
    ("recall@10", lambda r: r["aggregate"].get("recall_at_10", 0)),
    ("navigation h@5", lambda r: r.get("by_type", {}).get("navigation", {}).get("hit_at_5", 0)),
    ("concept h@5", lambda r: r.get("by_type", {}).get("concept", {}).get("hit_at_5", 0)),
    ("behavior h@5", lambda r: r.get("by_type", {}).get("behavior", {}).get("hit_at_5", 0)),
    ("negative pass", lambda r: r.get("by_type", {}).get("negative", {}).get("negative_pass_rate", 0)),
]


def print_deltas(reports: list[dict], labels: list[str]) -> None:
    base = reports[0]
    base_label = labels[0]

    print()
    print(f"Deltas relative to '{base_label}' (percentage points):")

    metric_width = max(len(name) for name, _ in DELTA_METRICS)
    for name, getter in DELTA_METRICS:
        base_val = getter(base)
        cells = []
        for label, r in zip(labels[1:], reports[1:]):
            v = getter(r)
            d = (v - base_val) * 100
            sign = "+" if d >= 0 else ""
            cells.append(f"{label}: {sign}{d:.1f}pp")
        print(f"  {name.ljust(metric_width)}  " + "  ".join(cells))


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Compare ragcodepilot eval JSON reports.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog=__doc__,
    )
    parser.add_argument(
        "reports",
        nargs="+",
        help="One or more eval JSON files (the first is the baseline for deltas)",
    )
    parser.add_argument(
        "--labels",
        help="Comma-separated labels, one per report (defaults to filename stem)",
    )
    args = parser.parse_args()

    paths = [Path(p) for p in args.reports]
    for p in paths:
        if not p.exists():
            print(f"error: {p} not found", file=sys.stderr)
            return 1

    reports = [load_report(p) for p in paths]

    if args.labels:
        labels = [s.strip() for s in args.labels.split(",")]
        if len(labels) != len(reports):
            print(
                f"error: --labels has {len(labels)} entries, need {len(reports)}",
                file=sys.stderr,
            )
            return 1
    else:
        labels = [p.stem for p in paths]

    print_metadata(reports, labels)
    rows = [aggregate_row(label, r) for label, r in zip(labels, reports)]
    print_table(rows)

    if len(reports) > 1:
        print_deltas(reports, labels)

    return 0


if __name__ == "__main__":
    sys.exit(main())
