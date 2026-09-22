# AGENTS.md

Rules for any agent, human or otherwise, contributing to this
repository.

## Declare LLM assistance

Any commit an LLM took part in must carry this trailer, written exactly
like this:

```
Assisted-by: LLM
```

The declaration is mandatory whatever the share of the work, from a
reviewed suggestion to a fully generated diff, and it is not a
judgement call. The form is fixed and names no vendor, product or
model: what a reader needs is that a machine took part and that a human
signed for it, not which company sold the tool. It replaces the former
`Co-Authored-By` trailer, which claimed an authorship an LLM cannot
hold in law and credited a vendor instead of informing anyone. The
`Signed-off-by` trailer carries the responsibility, and that
responsibility is always human.

This project uses AI assistance and does not hide it. Someone reading
the history deserves to know where a change came from.
