+++
name = "quality-review"
description = "Review merge quality and track per-worker trends"
version = 1

[gate]
type = "cooldown"
duration = "6h"

[tracking]
labels = ["plugin:quality-review", "category:quality"]
digest = true

[execution]
timeout = "5m"
notify_on_failure = true
severity = "medium"
+++

# Quality Review — Trend Analysis

This plugin runs every 6h during Deacon patrol. It analyzes quality-review result
wisps recorded by the Refinery during merges, computes per-worker trends, and
alerts on quality breaches.

## Step 1: Query recent quality-review results

Fetch all quality-review result wisps from the last 24 hours:

```bash
bd list --json --all -l type:plugin-run,plugin:quality-review-result --created-after=-24h
```

If no results are found, record a run wisp and stop:

```bash
gt plugin record-run --plugin quality-review --result success \
  --title "quality-review: No results in last 24h" \
  --description "No quality-review results in last 24h. Nothing to analyze." >/dev/null 2>&1 || true
```

## Step 2: Compute per-worker trends

Parse the wisp labels to extract per-worker data. Each result wisp has labels:
- `worker:<polecat-name>`
- `rig:<rig-name>`
- `score:<0.0-1.0>`
- `recommendation:<approve|request_changes>`

For each worker, compute:
- **Average score** across all results in window
- **Rejection rate**: count of `recommendation:request_changes` / total
- **Trend direction**: Compare first-half avg vs second-half avg of the window
  - Difference > 0.05: `improving`
  - Difference < -0.05: `declining`
  - Otherwise: `stable`

## Step 3: Classify worker status

Apply thresholds to each worker's average score:
- **OK**: avg >= 0.60
- **WARN**: 0.45 <= avg < 0.60
- **BREACH**: avg < 0.45

## Step 4: Alert on breaches

For each worker in BREACH status, send an alert:

```bash
gt mail send mayor/ -s "Quality BREACH: <worker>" -m "Worker: <worker>
Rig: <rig>
Avg Score: <avg>
Reviews: <count>
Rejection Rate: <rate>%
Trend: <improving|stable|declining>

Action: Review recent merges from this worker for quality issues."
```

Also escalate:

```bash
gt escalate "Quality BREACH: <worker> (avg: <avg>)" \
  --severity medium \
  --reason "Worker <worker> in rig <rig> has avg quality score <avg> over <count> reviews"
```

## Step 5: Record run result

Record a summary wisp for this plugin run:

```bash
gt plugin record-run --plugin quality-review --result success \
  --title "quality-review: Analyzed <N> workers over <M> reviews" \
  --description "Analyzed <N> workers over <M> reviews. <B> breaches, <W> warnings." >/dev/null 2>&1 || true
```

If any step fails unexpectedly, record a failure wisp and escalate:

```bash
gt plugin record-run --plugin quality-review --result failure \
  --title "quality-review: FAILED" \
  --description "<error description>" >/dev/null 2>&1 || true

gt escalate "Plugin FAILED: quality-review" \
  --severity medium \
  --reason "$ERROR"
```

---

## How scores get recorded (reference)

This plugin does NOT record scores itself. The Refinery records result wisps during
merges via the `quality-review` formula step. Each merge produces a wisp like:

```bash
gt plugin record-run --plugin quality-review-result --result success --rig <rig-name> \
  --label worker:<polecat-name> --label score:0.85 --label recommendation:approve \
  --title "quality-review: Score 0.85, approve" \
  --description "Score: 0.85, approve. Issues: 1 minor (style)" >/dev/null 2>&1 || true
```

This creates the data that Step 1 queries.
