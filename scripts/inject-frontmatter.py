#!/usr/bin/env python3
"""
inject-frontmatter.py

Walks one or more source directories, finds all .md files, strips existing
YAML front-matter, injects randomised front-matter, and writes results to
a flat output directory.

All I/O is done in a multiprocessing pool — one worker per CPU core.

Usage (called by build-reference-library.sh):
    inject-frontmatter.py --out <dest_dir> [--limit N] <src_dir> [<src_dir> ...]
"""

import argparse
import hashlib
import os
import random
import re
import sys
from datetime import date, timedelta
from pathlib import Path

# ── Vocabulary pools ──────────────────────────────────────────────────────────

STATUSES = ["inbox", "verified", "deprecated", "contested", "superseded"]
CONFIDENCES = ["low", "medium", "high"]
TYPES = ["observation", "pattern", "constraint", "decision", "assumption", "synthesis"]
SCOPES = ["project", "cross-project"]

PROJECTS = [
    "note-searcher", "api-gateway", "auth-service", "billing-pipeline",
    "data-warehouse", "ml-platform", "monitoring-stack", "ci-cd-infra",
    "customer-portal", "analytics-engine", "search-indexer", "notification-service",
]

DOMAINS = [
    "kubernetes", "azure", "cloud", "devops", "networking", "security",
    "storage", "compute", "monitoring", "observability", "ci-cd", "helm",
    "docker", "containers", "microservices", "api", "authentication",
    "infrastructure", "iac", "terraform", "ansible", "linux", "bash",
    "python", "go", "golang", "databases", "postgres", "redis", "kafka", "grpc",
    "http", "tls", "dns", "loadbalancing", "autoscaling", "rbac",
    "namespaces", "ingress", "service-mesh", "istio", "ebpf", "gitops",
    "argocd", "fluxcd", "cost-optimisation", "backup", "disaster-recovery",
    "compliance", "logging", "tracing", "alerting", "sre", "platform-eng",
    "error-handling", "performance", "scalability", "resilience",
]

AGENTS = [
    "coder-agent", "review-agent", "doc-agent", "test-agent", "refactor-agent",
    "security-agent", "perf-agent", "migration-agent", "integration-agent",
]

ARTIFACTS = [
    "pkg/auth/handler.go", "cmd/server/main.go", "internal/service/user.go",
    "api/openapi.yaml", "docs/architecture.md", "README.md",
    "terraform/main.tf", "k8s/deployment.yaml", "docker-compose.yml",
    "scripts/migrate.sh", ".github/workflows/ci.yml", "Dockerfile",
]

_DATE_START = date(2019, 1, 1)
_DATE_RANGE = (date(2025, 12, 31) - _DATE_START).days

# Precompiled regexes (compiled once per process, reused across files)
_RE_FRONTMATTER = re.compile(r"^\s*---\s*\n.*?\n---\s*\n", re.DOTALL)
_RE_H1 = re.compile(r"^#\s+(.+)", re.MULTILINE)
_RE_CODE = re.compile(r"`([^`]+)`")
_RE_LINK = re.compile(r"\[([^\]]+)\]\([^)]+\)")

# Regex to identify non-English content directories
# Matches patterns like: /content/<lang-code>/ where lang is not 'en'
# Examples: /content/zh-cn/, /content/ja/, /content/fr/, etc.
_RE_NON_ENGLISH = re.compile(r'/content/(bn|de|es|fa|fr|hi|id|it|ja|ko|pl|pt-br|ru|uk|vi|zh-cn)(/|$)')

# ── Per-file processing ───────────────────────────────────────────────────────

def _seed(path: str) -> int:
    """Deterministic seed from path so re-runs produce identical front-matter."""
    return int(hashlib.md5(path.encode(), usedforsecurity=False).hexdigest(), 16) % (2 ** 32)


def _strip_frontmatter(text: str) -> str:
    return _RE_FRONTMATTER.sub("", text, count=1).lstrip()


def _derive_title(text: str, fallback: str) -> str:
    m = _RE_H1.search(text)
    if m:
        t = _RE_CODE.sub(lambda mo: mo.group(1), m.group(1).strip())
        t = _RE_LINK.sub(r"\1", t)
        return t[:120]
    return fallback.replace("-", " ").replace("_", " ").title()


def _frontmatter(rng: random.Random, title: str) -> str:
    domains = rng.sample(DOMAINS, rng.randint(1, 3))
    # Tags can be 0-5 items from the domains pool
    num_tags = rng.choices([0, 1, 2, 3, 4, 5], weights=[2, 3, 4, 3, 2, 1])[0]
    tags = rng.sample(DOMAINS, min(num_tags, len(DOMAINS))) if num_tags > 0 else []
    
    created = (_DATE_START + timedelta(days=rng.randint(0, _DATE_RANGE))).isoformat()
    # updated is usually same as created, sometimes a few days later
    updated_delta = rng.choices([0, 0, 0, rng.randint(1, 30)], weights=[7, 1, 1, 1])[0]
    updated = (date.fromisoformat(created) + timedelta(days=updated_delta)).isoformat()
    review_by = (date.fromisoformat(created) + timedelta(days=rng.randint(60, 120))).isoformat()
    
    # Some notes might be superseded
    superseded_by = "" if rng.random() > 0.1 else rng.choice(["note-123", "note-456", "note-789"])
    
    tags_str = ", ".join(tags) if tags else ""
    
    return (
        "---\n"
        f'title: "{title}"\n'
        f"created: {created}\n"
        f"updated: {updated}\n"
        f"status: {rng.choice(STATUSES)}\n"
        f"confidence: {rng.choice(CONFIDENCES)}\n"
        f"type: {rng.choice(TYPES)}\n"
        f"scope: {rng.choice(SCOPES)}\n"
        f"project: {rng.choice(PROJECTS)}\n"
        f"domain: [{', '.join(domains)}]\n"
        f"source-agent: {rng.choice(AGENTS)}\n"
        f"source-artifact: \"{rng.choice(ARTIFACTS)}\"\n"
        f"review-by: {review_by}\n"
        f"requires-human-review: {str(rng.random() > 0.7).lower()}\n"
        f"superseded-by: \"{superseded_by}\"\n"
        f"tags: [{tags_str}]\n"
        "---\n\n"
    )


def _process_file(args: tuple) -> str | None:
    """
    Worker function — runs in a subprocess pool.
    Returns dest path on success, None on skip.
    """
    src_str, dest_str = args
    src, dest = Path(src_str), Path(dest_str)

    try:
        raw = src.read_bytes().decode("utf-8", errors="replace")
    except OSError:
        return None

    body = _strip_frontmatter(raw)
    if len(body.strip()) < 50:
        return None

    rng = random.Random(_seed(src_str))
    title = _derive_title(body, src.stem)
    output = _frontmatter(rng, title) + body

    try:
        dest.write_text(output, encoding="utf-8")
    except OSError:
        return None

    return dest_str


# ── File discovery ────────────────────────────────────────────────────────────

def _collect_files(src_dirs: list[str], out_dir: Path, limit: int) -> list[tuple[str, str]]:
    """
    Walk each source directory, build (src, dest) pairs.
    dest filename = <repo-name>__<sanitised-relative-path>.md
    """
    pairs: list[tuple[str, str]] = []
    seen_dests: set[str] = set()

    for src_dir in src_dirs:
        src_path = Path(src_dir)
        repo_name = src_path.name

        # os.walk is faster than Path.rglob for large trees
        for dirpath, _, filenames in os.walk(src_dir, followlinks=False):
            for fn in filenames:
                if not fn.endswith(".md"):
                    continue
                full = os.path.join(dirpath, fn)
                
                # Skip non-English content (only keep English or no-language-code files)
                if _RE_NON_ENGLISH.search(full):
                    continue
                
                rel = os.path.relpath(full, src_dir)
                dest_name = repo_name + "__" + rel.replace(os.sep, "__").replace(" ", "_")
                # Avoid collisions (shouldn't happen, but be safe)
                base, ext = os.path.splitext(dest_name)
                unique = dest_name
                n = 1
                while unique in seen_dests:
                    unique = f"{base}__{n}{ext}"
                    n += 1
                seen_dests.add(unique)
                pairs.append((full, str(out_dir / unique)))

                if limit and len(pairs) >= limit:
                    return pairs

    return pairs


# ── Entry point ───────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("src_dirs", nargs="+")
    parser.add_argument("--out", required=True)
    parser.add_argument("--limit", type=int, default=0)
    args = parser.parse_args()

    out_dir = Path(args.out)
    out_dir.mkdir(parents=True, exist_ok=True)

    print("🔍 Discovering markdown files...")
    pairs = _collect_files(args.src_dirs, out_dir, args.limit)
    total = len(pairs)
    print(f"   Found {total:,} files — starting parallel processing...")

    cpu_count = os.cpu_count() or 4
    # More workers than CPUs — work is I/O-bound, threads are ideal
    workers = min(cpu_count * 4, 64)
    print(f"   Workers: {workers} (CPUs: {cpu_count})")

    processed = 0
    skipped = 0
    report_every = max(1000, total // 20)  # progress every ~5%

    from concurrent.futures import ThreadPoolExecutor, as_completed
    with ThreadPoolExecutor(max_workers=workers) as pool:
        futures = {pool.submit(_process_file, p): p for p in pairs}
        for future in as_completed(futures):
            if future.result():
                processed += 1
            else:
                skipped += 1
            done = processed + skipped
            if done % report_every == 0:
                pct = done / total * 100
                print(f"   {done:,}/{total:,} ({pct:.0f}%) — processed: {processed:,}  skipped: {skipped:,}")

    print(f"\n✅ Processed: {processed:,}  ⏭️  Skipped: {skipped:,}")


if __name__ == "__main__":
    main()
