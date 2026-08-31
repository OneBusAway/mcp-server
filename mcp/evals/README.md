# MCP evaluations

The evaluation system has three separate layers:

```text
suite JSON
   ├─ make eval-test ──> declared calls ──> real handlers ──> fixed OBA fixture
   └─ make eval-live ──> model chooses calls ──> real handlers ──> fixed OBA fixture
                                                   │
                                                   └─ transcript ──> eval-score
```

`eval-test` detects server and expectation regressions without a model.
`eval-live` detects model tool-selection, argument, call-efficiency, and answer
regressions. A live eval still uses fixed transit data; only the model endpoint
is external. This makes runs comparable and prevents a changing transit feed
from changing the expected result.

`all-tools-v1.json` is the versioned, model-agnostic baseline for the entire
MCP catalog. Every registered tool has a success case and a controlled failure
case. `scenarios-v1.json` adds ambiguous names, empty arrivals, stale data,
rate limiting, malformed upstream data, malicious values, and a composite
rider workflow.

CI compares the suites with the actual tool registration. For direct IDs,
`prompt_argument_keys` requires expected values to appear as complete IDs in
the prompt. Raw IDs should be delimited from prose with backticks so a stale
expected argument cannot silently pass.

`make eval-test` does two things:

1. Validates suite structure and catalog coverage.
2. Executes every declared call against deterministic fixtures. Success cases
   must return a `SuccessEnvelope`; failures must return the declared stable
   error code. Composite cases execute each expected call in order.

This runner verifies handlers and declared expectations. It does not ask a
model to choose tools.

## Files

- `all-tools-v1.json`: success and controlled-failure coverage for all tools.
- `scenarios-v1.json`: multi-tool, empty, stale, adversarial, and upstream
  failure behavior.
- `evaluations.go`: suite and case types plus suite loading.
- `evaluations_test.go`: suite structure, coverage, and prompt/argument guards.
- `fixtures.go`: shared deterministic OBA behavior used by both runners.
- `runner_test.go`: executes the calls declared in each suite against the real
  handlers.
- `live.go`: OpenAI-compatible model loop, actual handler dispatch, and safe
  transcript capture.
- `live_test.go`: end-to-end runner test with a fake model endpoint.
- `scoring.go`: transcript format, deterministic scoring, and secure writing.
- `scoring_test.go`: scorer and transcript contract tests.
- `cmd/evalrun`: command used by `make eval-run` and `make eval-live`.
- `cmd/evalscore`: command used by `make eval-score`.

## Model/client scoring

The direct live runner uses a non-streaming OpenAI-compatible chat-completions
tool-call contract. For each case it:

1. Starts the case's fixed OBA fixture.
2. Registers the selected real MCP tool catalog.
3. Sends the prompt, sorted tool definitions, and JSON Schemas to the model.
4. Validates and invokes each selected tool through its real handler.
5. Sends canonical structured tool content back to the model.
6. Stops at the final answer or configured token, round, and call limits.
7. Records tool calls, public structured MCP results/error codes, and the final
   answer so semantic claims can be compared with the actual data.

Provider failures preserve completed cases and add a safe `runner_error` to the
failed case before the CLI writes the partial transcript. Safety-limit and
unknown-tool conditions are also runner errors, not fabricated public MCP
errors, and always fail deterministic scoring.

Provider credentials are read only from `EVAL_API_KEY`. They are never accepted
as a command-line flag, written to the transcript, or included in provider
errors. Transcripts are written atomically with owner-only permissions and are
ignored by Git.

A model or client runner should produce a credential-free transcript:

```json
{
  "suite_version": "scenarios-v1",
  "profile": {
    "id": "local-small",
    "client": "direct-mcp",
    "provider": "ollama",
    "model": "model-name"
  },
  "cases": [
    {
      "id": "no-upcoming-arrivals",
      "tool_calls": [
        {
          "name": "get_arrivals_for_stop",
          "arguments": { "stop_id": "test_1013" }
        }
      ],
      "response": "There are no upcoming arrivals."
    }
  ]
}
```

For a controlled tool failure, record the public result code as
`"error_code": "UPSTREAM_RATE_LIMITED"` on that observed call.

Score it from `mcp/`:

```sh
make eval-score TRANSCRIPT=/path/to/transcript.json
```

Run and score a local Ollama model:

```sh
EVAL_API_BASE_URL=http://127.0.0.1:11434/v1 \
EVAL_MODEL=your-tool-capable-model \
EVAL_PROVIDER=ollama \
EVAL_PROFILE_ID=local-your-model \
make eval-live
```

Run and score an OpenAI-compatible hosted model:

```sh
EVAL_API_BASE_URL=https://api.openai.com/v1 \
EVAL_API_KEY=your-key \
EVAL_MODEL=your-model \
EVAL_PROVIDER=openai \
EVAL_PROFILE_ID=frontier-your-model \
make eval-live
```

Use `EVAL_SUITE=evals/all-tools-v1.json` to score the catalog baseline. The
`EVAL_TRANSCRIPT=evals/transcripts/name.json` setting keeps multiple runs. The
MCP tool profile defaults to `all`; set `EVAL_TOOL_PROFILE=rider` only when the
profile under evaluation intentionally exposes that smaller catalog. An
optional client-specific prompt can be supplied directly with
`go run ./cmd/evalrun -system-prompt path ...`. The scorer checks exact ordered
tool selection, semantic JSON arguments, public tool-result error codes, call
budgets, suite/case identity, runner failures, and optional required/forbidden
response terms.
Invalid-argument cases may explicitly accept a zero-call refusal as safe model
behavior; the deterministic handler runner still executes the declared call
and verifies server-side validation and its public error code.
It marks `expected_outcome` for explicit human or configured-judge review; a
deterministic pass alone is not a semantic pass.

"Local" means a model served on your machine, usually through Ollama or
llama.cpp. "Frontier" means a strong current hosted model; it is a release-test
category, not special runner logic. Both use the same direct runner when their
endpoint supports the OpenAI-compatible contract, which makes the comparison
fair. The UI integration path remains separate because it must test the UI's
own system prompt, tool filtering, streaming parser, and dispatch behavior.

Before a release candidate, run the scenarios through at least a smaller/local
model, a frontier model, and the UI integration path against the same fixture.
Record the client/provider/model, fixture revision, suite version, deterministic
pass rate, semantic review, and failures in the release issue. Never commit
credentials, raw provider payloads, or transcripts; `evals/transcripts/` and
`evals/results/` are ignored for local runs.
