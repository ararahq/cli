# RFC NNNN: <Title>

- **Status**: Draft
- **Author(s)**: Your Name <email-or-handle>
- **Created**: YYYY-MM-DD
- **Updated**: YYYY-MM-DD
- **Related**: links to PRs, issues, prior RFCs

## Summary

One paragraph (2–4 sentences) explaining the proposal. A reader should be able to skim this and decide whether to keep reading.

## Motivation

Why are we doing this? What's broken or missing today? Who is the user (CLI human, automation, agent, plugin author)? What's the cost of *not* doing it?

Keep this section concrete. Replace "users want X" with "I tried to do X and the friction was Y" or "issue #NN documents this pain".

## Guide-level explanation

Explain the proposal **as a user would experience it**. Show command examples, file contents, expected output. If the change is invisible to the user, explain it in terms a contributor would experience instead.

```bash
$ arara <new-command> --new-flag value
✔ <expected output>
```

Cover:

- The happy path
- One or two edge cases the user will actually hit
- How errors look

## Reference-level explanation

The implementation-level details. A reviewer should be able to estimate the work after reading this.

- Affected packages: `internal/foo/`, `internal/bar/`, ...
- New types / interfaces / functions, with signatures
- Migration story for existing data, configs, or callers
- Test plan (unit, integration, e2e)
- Performance, memory, or security implications

## Drawbacks

What this proposal makes worse, costs to maintain, paths it forecloses. Be honest — every RFC has drawbacks.

## Rationale and alternatives

Why this design? What other approaches did you consider?

- Alternative A: <description> — rejected because <reason>
- Alternative B: <description> — rejected because <reason>
- Do nothing: what happens if we punt?

## Prior art

How does this work in other tools? Link to docs, blog posts, code in `git`, `kubectl`, `gh`, `claude-code`, `stripe-cli`, `twilio-cli`, etc. Briefly note where they got it right and where they made mistakes we'd avoid.

## Unresolved questions

Bullet list of things we **don't** know yet. These should be answered before the RFC moves to FCP, or explicitly punted to a follow-up.

- [ ] Question 1
- [ ] Question 2

## Future possibilities

Hooks for future work this RFC enables but doesn't itself ship. Keeps reviewers from feature-creeping the current proposal.
