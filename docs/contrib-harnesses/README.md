# Contributor Harnesses

Reference examples of **role directives** and **formula overlays** that
contributors and operators can drop into their own Gas Town setup to customize
agent behavior — without patching the framework.

These examples are not active by default. They are starting points you copy
into your own `~/gt/<rig>/directives/` or `~/gt/<rig>/formula-overlays/`
directory and adapt to your rig's needs.

See [`docs/design/directives-and-overlays.md`](../design/directives-and-overlays.md)
for the design of the extension surface (how directives and overlays are
loaded, injection points, precedence, validation). The canonical prime-time
reference is `~/gt/docs/PRIMING.md` § "Role Directives and Formula Overlays".

## Available harnesses

| Harness | What it does |
|---------|--------------|
| [`polecat-pr-flow/`](polecat-pr-flow/) | Makes polecats open a GitHub PR for their branch before running `gt done`. For rigs that use a PR-review workflow instead of the canonical Refinery merge-queue flow. |

## Scope

Each harness is intentionally small and focused — enough to show the shape of
a directive or overlay without covering every edge case. You are expected to
read it, copy it, and adapt it.

Harnesses do **not**:

- Modify Go source or formula source-of-truth files
- Change default agent behavior for anyone who hasn't opted in
- Replace `gt doctor` validation — run `gt doctor` after installing to confirm
  overlays are healthy

## Contributing a new harness

If you've built a directive or overlay that other operators might want, open a
PR adding a new directory here with:

- `README.md` — what it does, how to install, how to verify it's active
- The directive (`<role>.md`) and/or overlay (`<formula>.toml`) files
- Keep it short. A harness is a worked example, not a complete product.
