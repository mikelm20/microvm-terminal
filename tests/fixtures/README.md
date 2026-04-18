# Claude Code stream-json fixtures

Corpus of captured JSONL outputs from
`claude --output-format stream-json --verbose --print "<prompt>"`
used by tests/learner-bot and the parser package to validate the
spine without a live Claude token.

## Regenerate

Pre-reqs:

- `claude` CLI authenticated on the recording machine (run
  `claude setup-token` or log in interactively once).
- Env var `CLAUDE_CODE_OAUTH_TOKEN` exported for non-interactive
  captures.
- A cwd that matches the lesson sandbox (e.g., `~/empresa-prueba`).

For a given lesson step, run:

```
cd ~/empresa-prueba
CLAUDE_CODE_OAUTH_TOKEN=$(cat ~/.claude/.credentials.json | jq -r .token) \
  claude --output-format stream-json --input-format stream-json --verbose \
  --dangerously-skip-permissions < fixtures/input/m2-leer-departamento-1.jsonl \
  > fixtures/claude/m2-leer-departamento-1.jsonl
```

Naming convention: `m<module>-<step-id>-<attempt>.jsonl`. Attempts
1..N per prompt are captured to cover successful, noisy, and
error branches where they matter for predicate evaluation.

## Hand-crafted fixtures

Where live Claude is unavailable (CI) we carry hand-crafted
fixtures that replicate the wire shape documented in
`vm-image/claude-wrap/internal/streamjson/types.go`. They are
enough for the parser + evaluator tests; full end-to-end
validation still requires live replay.

## Current corpus

| File | Lesson | Step | Variant |
|---|---|---|---|
| m2-leer-departamento-1.jsonl | m2 | leer-departamento | Read success |
| m2-leer-departamento-2.jsonl | m2 | leer-departamento | multi-file Read |
| m2-corregir-1.jsonl | m2 | corregir-sobre-la-marcha | short reply |
| m2-pedir-ambicioso-1.jsonl | m2 | pedir-algo-ambicioso | Read+Write chain |
| m4-parte-matinal-1.jsonl | m4 | parte-matinal | Read inbox + calendar |
| m4-brief-reunion-1.jsonl | m4 | brief-pre-reunion | Read persona |
| m5-ver-subagentes-1.jsonl | m5 | ver-subagentes-vacios | SlashCommand |
| m5-crear-jefe-agenda-1.jsonl | m5 | crear-jefe-de-agenda | Bash mkdir + Write |
| m5-invocar-1.jsonl | m5 | invocar-jefe-de-agenda | Task w/ subagent |
