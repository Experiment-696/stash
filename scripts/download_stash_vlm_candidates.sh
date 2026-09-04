#!/usr/bin/env bash
set -Eeuo pipefail

# Download the Phase 2 VLM candidate slate. Large checkpoints are replaced by
# a Q4 (or, when necessary, Q3) GGUF package selected from a restricted set of
# publishers. The requested destination spelling is intentionally preserved.

DEST="${DEST:-/mnt/ai_models/stash_canidates}"
MAX_FULL_GIB="${MAX_FULL_GIB:-12}"
MAX_QUANT_GIB="${MAX_QUANT_GIB:-12.0}"
MIN_QUANT_GIB="${MIN_QUANT_GIB:-7.0}"
HF_HOME="${HF_HOME:-$DEST/.hf-cache}"
PLAN="$DEST/download-plan.tsv"
MANIFEST="$DEST/download-manifest.tsv"

export DEST MAX_FULL_GIB MAX_QUANT_GIB MIN_QUANT_GIB HF_HOME PLAN

die() { printf 'error: %s\n' "$*" >&2; exit 1; }
log() { printf '[stash-vlm] %s\n' "$*"; }

command -v python3 >/dev/null 2>&1 || die "python3 is required"
command -v hf >/dev/null 2>&1 || die "Hugging Face CLI is required: python3 -m pip install --user 'huggingface_hub[cli]>=0.27'"

mkdir -p "$DEST" "$HF_HOME"
[[ -d "$DEST" && -w "$DEST" ]] || die "destination is not writable: $DEST"

if [[ -z "${HF_TOKEN:-}" ]]; then
  log "HF_TOKEN is not set. Gated models (Gemma/Llama) may be skipped unless 'hf auth login' is already configured."
fi

log "Building a size-checked download plan (no model files downloaded yet)"

python3 - <<'PY'
import os
import re
import sys
from pathlib import Path

try:
    from huggingface_hub import HfApi
except ImportError:
    sys.exit("huggingface_hub Python package is required")

GIB = 1024 ** 3
dest = Path(os.environ["DEST"])
plan_path = Path(os.environ["PLAN"])
max_full = float(os.environ["MAX_FULL_GIB"]) * GIB
max_quant = float(os.environ["MAX_QUANT_GIB"]) * GIB
min_quant = float(os.environ["MIN_QUANT_GIB"]) * GIB

models = [
    ("pixtral-12b", "mistralai/Pixtral-12B-2409"),
    ("qwen3-vl-8b", "Qwen/Qwen3-VL-8B-Instruct"),
    ("minicpm-v-4.5", "openbmb/MiniCPM-V-4_5"),
    ("internvl3.5-8b", "OpenGVLab/InternVL3_5-8B"),
    ("kimi-vl-a3b", "moonshotai/Kimi-VL-A3B-Instruct"),
    ("qwen3-vl-4b", "Qwen/Qwen3-VL-4B-Instruct"),
    ("mistral-small-3.1-24b", "mistralai/Mistral-Small-3.1-24B-Instruct-2503"),
    ("molmo-7b-d", "allenai/Molmo-7B-D-0924"),
    ("gemma-3-12b", "google/gemma-3-12b-it"),
    ("llama-3.2-11b-vision", "meta-llama/Llama-3.2-11B-Vision-Instruct"),
]

# Quantized code/weights are executable supply-chain inputs. Prefer upstream
# publishers, then these established quantization publishers; reject all others.
trusted_publishers = {
    "Qwen", "openbmb", "OpenGVLab", "moonshotai", "mistralai",
    "allenai", "google", "meta-llama", "bartowski", "unsloth",
}

api = HfApi(token=os.environ.get("HF_TOKEN"))

def siblings(repo):
    info = api.model_info(repo, files_metadata=True)
    return info, [(s.rfilename, int(s.size or 0)) for s in info.siblings]

def weight_bytes(files):
    extensions = (".safetensors", ".bin", ".pth", ".pt", ".gguf")
    return sum(size for name, size in files if name.lower().endswith(extensions))

def quant_groups(files):
    """Return complete single-file or split GGUF groups by quant label."""
    groups = {}
    for name, size in files:
        lower = name.lower()
        if not lower.endswith(".gguf") or "mmproj" in lower:
            continue
        match = re.search(r"(?i)(q4_k_m|q4_k_s|q4_0|q3_k_l|q3_k_m|q3_k_s)", name)
        if not match:
            continue
        label = match.group(1).upper()
        # Remove split suffix so every shard lands in one package group.
        stem = re.sub(r"-\d{5}-of-\d{5}(?=\.gguf$)", "", name, flags=re.I)
        key = (label, stem)
        groups.setdefault(key, []).append((name, size))
    return groups

def choose_quant(base_repo):
    short = base_repo.split("/", 1)[1]
    queries = [f"{short} GGUF", short]
    seen = set()
    repos = []
    for query in queries:
        for model in api.list_models(search=query, limit=80):
            repo = model.id
            if repo in seen or "/" not in repo:
                continue
            seen.add(repo)
            owner = repo.split("/", 1)[0]
            if owner not in trusted_publishers or "gguf" not in repo.lower():
                continue
            # Require meaningful name overlap to avoid a similarly named model.
            tokens = [t for t in re.split(r"[-_.]", short.lower()) if len(t) >= 3]
            if sum(t in repo.lower() for t in tokens) < max(2, len(tokens) // 2):
                continue
            repos.append(repo)

    choices = []
    for repo in repos:
        try:
            info, files = siblings(repo)
        except Exception:
            continue
        mmproj = [(n, s) for n, s in files if n.lower().endswith(".gguf") and "mmproj" in n.lower()]
        for (label, _), shards in quant_groups(files).items():
            total = sum(s for _, s in shards)
            if not (min_quant <= total <= max_quant):
                continue
            # Prefer Q4_K_M, then other Q4, then Q3; within a label prefer the
            # largest package that remains inside the requested 7-12 GiB range.
            rank = {"Q4_K_M": 0, "Q4_K_S": 1, "Q4_0": 2,
                    "Q3_K_L": 3, "Q3_K_M": 4, "Q3_K_S": 5}[label]
            choices.append((rank, -total, repo, info.sha or "main", label, shards, mmproj))
    if not choices:
        return None
    return sorted(choices)[0]

rows = []
for slug, base_repo in models:
    try:
        base_info, base_files = siblings(base_repo)
        full_size = weight_bytes(base_files)
    except Exception as exc:
        rows.append((slug, "SKIP", base_repo, "", "", "", f"metadata error: {exc}"))
        continue

    if 0 < full_size <= max_full:
        rows.append((slug, "FULL", base_repo, base_info.sha or "main", "", "", f"{full_size/GIB:.2f} GiB weights"))
        continue

    choice = choose_quant(base_repo)
    if choice is None:
        rows.append((slug, "SKIP", base_repo, base_info.sha or "main", "", "", f"full weights {full_size/GIB:.2f} GiB; no trusted Q4/Q3 package in target range"))
        continue

    _, neg_total, repo, revision, label, shards, mmproj = choice
    selected = [name for name, _ in shards]
    # Download all projection files because runtime naming conventions vary.
    selected.extend(name for name, _ in mmproj)
    rows.append((slug, "GGUF", repo, revision, label, ";".join(selected), f"{-neg_total/GIB:.2f} GiB model weights"))

with plan_path.open("w", encoding="utf-8", newline="\n") as out:
    out.write("slug\tmode\trepo\trevision\tquant\tfiles\tnote\n")
    for row in rows:
        out.write("\t".join(str(v).replace("\t", " ").replace("\n", " ") for v in row) + "\n")

for row in rows:
    print(f"{row[0]:26} {row[1]:5} {row[2]:62} {row[4]:7} {row[6]}")
PY

printf '\n'
log "Plan written to $PLAN"
log "Review the plan above. Beginning resumable downloads in 5 seconds; press Ctrl-C to stop."
sleep 5

if [[ ! -f "$MANIFEST" ]]; then
  printf 'slug\tmode\trepo\trevision\tquant\tstatus\tpath\n' > "$MANIFEST"
fi

tail -n +2 "$PLAN" | while IFS=$'\t' read -r slug mode repo revision quant files note; do
  target="$DEST/$slug"
  if [[ "$mode" == "SKIP" ]]; then
    log "SKIP $slug: $note"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$mode" "$repo" "$revision" "$quant" "skipped" "$target" >> "$MANIFEST"
    continue
  fi

  mkdir -p "$target"
  log "Downloading $slug ($mode ${quant:-unquantized}) from $repo@$revision"
  args=(download "$repo" --revision "$revision" --local-dir "$target")
  if [[ "$mode" == "GGUF" ]]; then
    IFS=';' read -r -a selected_files <<< "$files"
    for file in "${selected_files[@]}"; do
      [[ -n "$file" ]] && args+=(--include "$file")
    done
    # Keep tokenizer/templates/configuration needed by common VLM runtimes.
    args+=(--include '*.json' --include '*.model' --include '*.txt' --include 'README*' --include 'LICENSE*')
  fi

  if hf "${args[@]}"; then
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$mode" "$repo" "$revision" "$quant" "downloaded" "$target" >> "$MANIFEST"
  else
    log "FAILED $slug; partial files are retained for resume"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$slug" "$mode" "$repo" "$revision" "$quant" "failed" "$target" >> "$MANIFEST"
  fi
done

log "Finished. Plan: $PLAN"
log "Manifest: $MANIFEST"
log "Model files target 7-12 GiB; actual use of the host's 24 GiB aggregate VRAM must still be measured with the intended runtime, GPU split, and image/context settings."
