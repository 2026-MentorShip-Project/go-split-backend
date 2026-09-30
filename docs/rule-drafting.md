# Rule drafting

A host describes how costs are shared in one sentence ("吃素的不用分肉錢，小孩算半份"). Gemini proposes tags, rules and member conditions. The host previews them and applies them in one step. Endpoint contracts are in [frontend-api-guide.md](frontend-api-guide.md#rule-drafting).

## How it fits together

1. `POST /events/{id}/rules/draft` loads the event's tags, rules and members, asks the configured `ruleassist.Generator` for a plan, and runs it through `ruleassist.Check`. It saves nothing. It runs outside `eventTransaction`, so a slow model call never holds the event lock.
2. The frontend shows the plan, with each member's share for every proposed rule.
3. `POST /events/{id}/rules/apply` checks the same plan again against the event as it is now, and writes it in one transaction. Nothing is written if any part no longer applies.

The generator is chosen at startup in `cmd/go-split-backend/server.go`:

| Setting | Generator |
|---|---|
| `RULE_DRAFT_FIXTURE=true` | `ruleassist.SampleFixture()`, canned plans for local development |
| `VERTEX_PROJECT`, `VERTEX_LOCATION`, `VERTEX_MODEL` all set | `VertexGenerator`: Gemini on Vertex AI |
| Neither | None: the draft endpoint answers 503 and the frontend hides the panel |

Chosen configuration: `VERTEX_LOCATION=global`, `VERTEX_MODEL=gemini-3.5-flash-lite`.

- `asia-east1` serves no Gemini text models; only embeddings, plus Claude Haiku 4.5 and Sonnet 4 ([locations](https://docs.cloud.google.com/gemini-enterprise-agent-platform/resources/locations?hl=zh-tw)).
- The global endpoint gives no data-residency guarantee. Member display names are sent to the model, which is accepted.

Safeguards:

- **Rate limit:** 5 drafts per host at once, then one every 12 s. Returns 429 with `Retry-After`. The limit is per Cloud Run instance.
- **Timeout:** 30 s, then 504.
- **Output:** capped at 2048 tokens, temperature 0.
- **Failures:** a blocked, cut-off or malformed answer is a 502, never a partial plan.
- **Budget:** the alert in `deploy/gcp/billing.tf`.

## Status

| Step | What | Where | State |
|---|---|---|---|
| Scaffold | `ruleassist` package, prompt, schema, `Check`, fixture | #23 | Merged |
| B1 | Draft endpoint | #28 | Merged |
| B4 | Apply endpoint (atomic) | #29 | Open |
| B2, B3 | Gemini call, timeout, rate limit | #30 | Open |
| F1, F2 | Draft API and panel | frontend #14 | Open |
| F3, F4 | Per-member preview, apply and discard | frontend #15 (stacked on #14, includes #9) | Open |
| B5 | Prompt quality check against real sentences | – | Not started, optional |
| Go live | Set the Vertex variables in production | – | Not done |

## Remaining steps, in order

1. **Merge the backend PRs: #29, then #30.**
   - They conflict in the "Rule drafting" section of `docs/frontend-api-guide.md`. Rebase whichever merges second, keep both sets of changes, and rerun `make swag` so `docs/docs.go` is regenerated.
   - Merging deploys the code, but drafting stays off in production until step 6.
2. **Merge the frontend PRs: #9, then #14, then #15.**
   - After #9 and #14 are in `main`, change #15's base to `main` so its diff shows only its own changes.
3. **Click through the UI against the fixture.**
   - Nobody has checked this in a browser yet.
   - Run the backend with `RULE_DRAFT_FIXTURE=true`, and the frontend with `API_URL=http://localhost:8080 npm run dev`.
   - As host, on the rules page, check:
     - Draft shows the plan and each member's share.
     - The amount box changes the preview.
     - Apply saves, and the rules refresh.
     - A `replace` on a tag with expenses asks first.
     - After a stale plan (for example, edit the rule in another tab, then apply), the panel shows the reasons and disables 套用.
     - Discard clears the draft.
4. **Make one live call locally.**
   - Log in with an account that can use Vertex AI in the project: `gcloud auth application-default login`.
   - Start the server: `VERTEX_PROJECT=<project> VERTEX_LOCATION=global VERTEX_MODEL=gemini-3.5-flash-lite go run ./cmd/go-split-backend` (plus the usual `DB_*` variables).
   - Draft a few real sentences through the UI or `curl`. Confirm the answers pass `Check` and match what the sentence asked for.
   - If the model rejects `responseJsonSchema` or keeps returning issues, fix that before going live.
5. **Optional, B5: a set of example sentences with expected plans.**
   - Write 20–30 real host sentences, each with the plan it should produce, run them through the model, `Check` and the engine, and compare.
   - Run it by hand or on a schedule, not in CI, because every run costs money.
   - Worth doing before real hosts rely on the feature, and whenever `prompt.md` or the model changes.
6. **Go live.**
   - In the backend repo's GitHub settings, set the repository variables `VERTEX_LOCATION=global` and `VERTEX_MODEL=gemini-3.5-flash-lite`.
   - Rerun the CD workflow (`workflow_dispatch`) so Terraform sets them on Cloud Run. `.github/workflows/cd.yaml` passes them as `TF_VAR_vertex_location` / `TF_VAR_vertex_model`; while either is unset, drafting stays off.
   - Nothing else changes. The service account already has `roles/aiplatform.user`, and Terraform already enables `aiplatform.googleapis.com`.
7. **Watch the first days.**
   - In Cloud Logging, look for `rule draft failed` and `rule draft timed out`.
   - Watch the budget alert.
   - To switch it off, clear either repository variable and redeploy.
