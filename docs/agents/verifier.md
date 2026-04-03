# verifier

**Phase:** Final — Verification

## Role

Final quality gate. Runs full typecheck and test suite on the feature branch.

## Input

- `comprehensive-review.md` — from Phase 7 (if ran)

## Output

- Verification result (pass/fail)

## Constraints

- Runs for all effort levels

## What It Does

1. Runs the full typecheck (`make typecheck`, `pnpm typecheck`, etc.)
2. Runs the full test suite (`make test`, `pnpm test`, etc.)
3. **Distinguishes pre-existing failures from new ones**
4. Fixes new failures before declaring the branch clean
5. If unfixable failures are found, reports to the user
