# Development

- Work in a separate git worktree on a `<TYPE>/<PURPOSE>` branch.
- Follow existing code, naming, formatting, and workflow conventions.
- Add meaningful unit tests for behavior changes and E2E tests for critical flows.
- Run relevant tests, race checks, and lint before finishing; report any blocked checks.
- Keep changes focused. Comments explain non-obvious reasons; avoid fluff.
- Commit in reviewable sections. Credit Codex with a coauthor trailer when it contributes.
- Preserve unrelated user changes. Remove the temporary worktree after committing.

# Delivery and deployment

- Push to a dedicated branch; never push directly to `main`.
- Use a PR targeting `main` and pass CI before merging.
- Test against disposable data and services; do not load-test production by default.
- Keep credentials out of code, logs, and test artifacts.
- Deploy through the existing deployment workflow; report the branch and validation results.
