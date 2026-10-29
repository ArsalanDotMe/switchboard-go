---
failed_layers: '' # set at runtime: comma-separated list of lenses that failed or returned empty
---

# Step 2: Review

## RULES

- All review subagents must run at the same model capability as the current session.
- Run subagents synchronously: launch them together as blocking calls awaited in this turn — never backgrounded or detached, never ending the turn to await results.

## INSTRUCTIONS

1. The review lenses are listed below. For each lens:
   - `Run only when` present and not satisfied by the current context (`{review_mode}`, `{plan_file}`, `{ticket_file}`, `{verbatim_intent}`) → skip the lens and tell the user which lens was skipped and why.
   - otherwise → the lens is active.

2. Announce skipped lenses first, then launch every active lens before handling any lens's result. Try running all active lenses simultaneously: substitute the runtime placeholders (`{diff_file}`, `{claims_file}`, `{plan_file}`, `{ticket_file}`, `{verbatim_intent}`) into each lens's instruction. `{diff_file}`, `{claims_file}`, `{plan_file}`, and `{ticket_file}` are paths: substitute the absolute path and let the lens read the file — a launch prompt never carries diff text, and the child's working directory is not yours. `{verbatim_intent}` is the Description, Requirements and Acceptance Criteria sections of `{ticket_file}` when it is the story file, with the epic's Requirements lines in its `covers`; when it is the epic file of an entry with no file, the entry's `title`, `description` and `verify` from `find`, with the epic's Requirements lines in the entry's `covers`; else the intent section of `{plan_file}` if it has one, else the user's or conversation's own words on what the change is for, substituted inline as text; a request to review something is not an intent. When an instruction launches a reviewer subagent, launch that child with the prompt text after placeholder substitution; do not load the reviewer instruction file yourself. For any other customized instruction, execute it as written. When running lenses as subagents, spawn every reviewer before reading or reacting to any of their output; begin collection only once all are launched.

#### Blind Hunter (`blind-hunter`)

Launch a context-free subagent with this prompt:

Conduct a review of CONTENT.
Look for what's missing, not only what's wrong.
Compute your finding floor N from the diff file's size: N = min(floor(sqrt(kB) + 1), 10), where kB is the file's size in kilobytes. State the arithmetic in one line, then find at least N issues to fix or improve.
Output a Markdown list of findings only — no severity, priority, or ranking.
If the content is empty, stop and say so.
If you have zero findings, re-check and keep thinking; do not stop with an empty list.

CONTENT: the unified diff at `{diff_file}`. Read that file — it is the content under review.

Review by inspection only. Do not modify code, run builds or tests, or invoke quality gates. Do not invoke any skill, and do not spawn subagents of your own — you are the reviewer. Return your findings as text in your final message; do not route them through any findings-reporting tool the host may offer.

#### Edge Case Hunter (`edge-case-hunter`)

Launch a context-free subagent with this prompt:

Read `/Users/eduardomenoncello/Documents/projects/switchboard-go/_bmad/render/bmad-code-review/switchboard-go-2cf1782a7724/e267213aef899f60e999/review-prompts/edge-case-hunter.md` completely and follow it as your review instructions.

claims_file (leave unread until your instructions call for it): {claims_file}

Review content: the unified diff at `{diff_file}`. Read that file — it is the content under review.

Review by inspection only. Do not modify code, run builds or tests, or invoke quality gates. Do not invoke any skill, and do not spawn subagents of your own — you are the reviewer. If the instruction file is unreadable, report that exact failure and stop. Return your findings as text in your final message; do not route them through any findings-reporting tool the host may offer.

#### Verification Gap Reviewer (`verification-gap`)

Launch a context-free subagent with this prompt:

Read `/Users/eduardomenoncello/Documents/projects/switchboard-go/_bmad/render/bmad-code-review/switchboard-go-2cf1782a7724/e267213aef899f60e999/review-prompts/verification-gap.md` completely and follow it as your review instructions.

Review content: the unified diff at `{diff_file}`. Read that file — it is the content under review.

Review by inspection only. Do not modify code, run builds or tests, or invoke quality gates. Do not invoke any skill, and do not spawn subagents of your own — you are the reviewer. If the instruction file is unreadable, report that exact failure and stop. Return your findings as text in your final message; do not route them through any findings-reporting tool the host may offer.

#### Intent Alignment Auditor (`intent-alignment`)

Run only when: an intent is available: the ticket's Requirements, the plan's intent section, or the user's or conversation's own words on what the change is for

Launch a subagent with this prompt:

You are an intent-alignment auditor. You have no other context about how this change was produced. Here is the verbatim intent this work started from:

{verbatim_intent}

The diff is the unified diff at `{diff_file}`. Read that file — it is the change under review.

Your task is strictly descriptive — do not prescribe additional work. Report: (1) the defensible readings of the intent, enumerated; (2) which reading this diff implements; (3) where the readings and the diff diverge — specifically, which surface the intent's expectations live at versus which surface the diff's changes and its tests exercise.

Review by inspection only. Do not modify code, run builds or tests, or invoke quality gates. Do not invoke any skill, and do not spawn subagents of your own — you are the reviewer. Return your findings as text in your final message; do not route them through any findings-reporting tool the host may offer.

3. If a lens's instruction requires subagents and none are available, for each such lens write that lens's child prompt beside `{plan_file}`, named after it with the lens id appended (in `/Users/eduardomenoncello/Documents/projects/switchboard-go/_bmad-output/initiative-multi-provider/` when there is no plan), with every file it points to — the diff, the claims, the reviewer instruction file — replaced inline by that file's contents, and every other line left exactly as written. That session shares no filesystem with this one, so its prompt has to stand alone; this is the only place you read a reviewer instruction file yourself. Then HALT. Ask the user to run each in a separate session (ideally a different LLM) and paste back the findings. When findings are pasted, treat them as those lenses' findings and resume from this point.

4. **Lens failure handling**: If any lens fails, times out, or returns empty results, append the lens's `name` to `failed_layers` (comma-separated) and proceed with findings from the remaining lenses.

5. Collect all findings from the completed lenses, keeping track of each finding's originating lens `id`.

## NEXT

Read fully and follow `/Users/eduardomenoncello/Documents/projects/switchboard-go/_bmad/render/bmad-code-review/switchboard-go-2cf1782a7724/e267213aef899f60e999/step-03-triage.md`
