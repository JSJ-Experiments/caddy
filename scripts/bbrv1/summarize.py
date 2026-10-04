#!/usr/bin/env python3
"""Summarize complete transfers, excluding range probes and protocol fallback."""
import collections
import json
import pathlib
import statistics
import sys

groups = collections.defaultdict(list)
for filename in sys.argv[1:]:
    for line in pathlib.Path(filename).read_text().splitlines():
        row = json.loads(line)
        if row["status"] == 200:
            groups[(row["label"], row["protocol"])].append(row)

print("| Scenario/mode | Protocol | Transfers | Aggregate MiB/s | Median seconds |")
print("|---|---|---:|---:|---:|")
for (label, protocol), rows in sorted(groups.items()):
    rate = sum(row["bytes"] for row in rows) / (1 << 20) / sum(row["seconds"] for row in rows)
    median = statistics.median(row["seconds"] for row in rows)
    print(f"| {label} | {protocol} | {len(rows)} | {rate:.2f} | {median:.3f} |")
