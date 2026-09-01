#!/usr/bin/env bash
# eval.sh — run MCP live evaluations with optional full system prompt
#
# Usage:
#   ./scripts/eval.sh [--full-prompt] [--suite <path>] [--out <path>]
#
# Required environment variables:
#   EVAL_API_BASE_URL   OpenAI-compatible API base URL (e.g. http://127.0.0.1:3000/v1)
#   EVAL_MODEL          Model name matching the server alias
#   EVAL_PROVIDER       Provider label recorded in the transcript (e.g. llama.cpp, openai)
#   EVAL_PROFILE_ID     Unique run identifier recorded in the transcript
#
# Optional environment variables:
#   EVAL_SUITE          Suite path (default: evals/scenarios-v1.json)
#   EVAL_API_KEY        API key for hosted providers — never logged or committed
#
# Flags:
#   --full-prompt       Inject the comprehensive transit_assistant system prompt
#                       instead of the default minimal eval guardrail.
#                       Use to measure production performance ceiling.
#                       Default (no flag) = baseline: tool descriptions only.
#   --suite <path>      Override EVAL_SUITE
#   --out <path>        Override auto-generated transcript path
#   --help              Show this message
#
# Transcript naming (auto-generated when --out is not set):
#   evals/transcripts/<EVAL_PROFILE_ID>[-full-prompt]-<suite-name>.json
#
# Examples:
#   # Baseline (tool descriptions only):
#   EVAL_API_BASE_URL=http://127.0.0.1:3000/v1 \
#   EVAL_MODEL=qwen3-8b-q4-no-think \
#   EVAL_PROVIDER=llama.cpp \
#   EVAL_PROFILE_ID=local-qwen3-8b \
#   ./scripts/eval.sh
#
#   # Full system prompt:
#   EVAL_API_BASE_URL=http://127.0.0.1:3000/v1 \
#   EVAL_MODEL=qwen3-8b-q4-no-think \
#   EVAL_PROVIDER=llama.cpp \
#   EVAL_PROFILE_ID=local-qwen3-8b \
#   ./scripts/eval.sh --full-prompt
#
#   # All-tools suite:
#   ./scripts/eval.sh --suite evals/all-tools-v1.json

set -euo pipefail

# Always run from mcp/ regardless of call site
cd "$(dirname "$0")/.."

FULL_PROMPT=false
SUITE="${EVAL_SUITE:-evals/scenarios-v1.json}"
OUT=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --full-prompt)  FULL_PROMPT=true; shift;;
    --suite)        SUITE="$2"; shift 2;;
    --out)          OUT="$2"; shift 2;;
    --help|-h)      awk 'NR>1 && /^[^#]/{exit} NR>1{sub(/^# ?/,""); print}' "$0"; exit 0;;
    *)              echo "Unknown flag: $1" >&2; exit 1;;
  esac
done

# Validate required env vars
missing=()
for var in EVAL_API_BASE_URL EVAL_MODEL EVAL_PROVIDER EVAL_PROFILE_ID; do
  [[ -z "${!var:-}" ]] && missing+=("$var")
done
if [[ ${#missing[@]} -gt 0 ]]; then
  echo "error: missing required environment variables: ${missing[*]}" >&2
  echo "       see ./scripts/eval.sh --help" >&2
  exit 1
fi

# Build profile ID and transcript path
SUITE_NAME=$(basename "$SUITE" .json)
PROFILE="${EVAL_PROFILE_ID}"
if $FULL_PROMPT; then
  PROFILE="${PROFILE}-full-prompt"
fi
OUT="${OUT:-evals/transcripts/${PROFILE}-${SUITE_NAME}.json}"

# Build evalrun command
CMD=(
  go run ./cmd/evalrun
  -suite   "$SUITE"
  -out     "$OUT"
  -base-url "$EVAL_API_BASE_URL"
  -model   "$EVAL_MODEL"
  -provider "${EVAL_PROVIDER}"
  -profile-id "${PROFILE}"
)

# Attach full system prompt when requested
if $FULL_PROMPT; then
  PROMPT_FILE="evals/system_prompt.txt"
  echo "→ dumping current system prompt to $PROMPT_FILE"
  go run ./scripts/dump-prompt > "$PROMPT_FILE"
  echo ""
  CMD+=(-system-prompt "$PROMPT_FILE")
fi

if $FULL_PROMPT; then
  MODE="full system prompt  (production ceiling)"
else
  MODE="baseline            (tool descriptions only)"
fi

echo "┌─────────────────────────────────────────────────────"
echo "│ suite     $SUITE"
echo "│ mode      $MODE"
echo "│ model     ${EVAL_MODEL} via ${EVAL_PROVIDER}"
echo "│ profile   ${PROFILE}"
echo "│ output    $OUT"
echo "└─────────────────────────────────────────────────────"
echo ""

"${CMD[@]}"

echo ""
echo "──────────────────────────────────────────────────────"
echo " score"
echo "──────────────────────────────────────────────────────"
set +e
go run ./cmd/evalscore -suite "$SUITE" -transcript "$OUT"
exit $?
