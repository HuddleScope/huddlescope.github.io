# Operations runbook — Publication heartbeat

## Manual dispatch

To manually trigger a refresh and optional Pages deployment:

```bash
# Basic dispatch (no deploy)
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=false

# Dispatch with deploy to Pages
gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=true
```

Check run status:

```bash
gh run list -R HuddleScope/huddlescope.github.io --limit 10
gh run view <RUN_ID> -R HuddleScope/huddlescope.github.io
```

## Reading the heartbeat

After dispatch completes, check the heartbeat file:

```bash
# View committed heartbeat
gh run download <RUN_ID> -R HuddleScope/huddlescope.github.io
cat web/data/heartbeat.json

# Or locally after checkout
cat web/data/heartbeat.json
```

Heartbeat status values:

- `ok`: Successfully completed ingestion and publication
- `partial`: Completed with warnings (some sources failed)
- `failed`: Critical failure (run did not complete)

## Deciding staleness

A heartbeat is stale when `published_at` is missing, unparseable, or older than 90 minutes.

Manual check:

```bash
# Check heartbeat staleness (exit 1 if stale)
dist/heartbeat check --file web/data/heartbeat.json --stale-after 90m

# View file content and assess
jq . web/data/heartbeat.json
```

## Turso outage recovery

If Turso is unreachable, follow this recovery:

1. **Dispatch a fresh run** (even if it will likely fail, it establishes a fresh heartbeat):

   ```bash
   gh workflow run refresh-news.yml -R HuddleScope/huddlescope.github.io --field publish=false
   ```

2. **Check the run logs**:

   ```bash
   gh run view <RUN_ID> -R HuddleScope/huddlescope.github.io --log-failed
   ```

   Look for Turso connection errors in the ingest step.

3. **If ingest fails**, the heartbeat file still exists with an error message and `status: failed`.

4. **Wait for Turso to recover**, then dispatch another run. The heartbeat will update with the next success.

5. **To restore homepage content**, if `homepage.json` was restored by the guard step:
   - Wait for a successful run to overwrite it
   - Or temporarily commit the last-known-good `homepage.json` from git history:
     ```bash
     git checkout HEAD~1 -- web/data/homepage.json
     git commit -m "Restore homepage.json from last-known-good"
     ```

## Alerting

GitHub Actions provides email/Slack notifications per workflow run. Configure in the repository settings:

- Workflow run success/failure notifications
- Individual job failure notifications

Heartbeat staleness is visually indicated in the workflow run output:

- Fresh heartbeat (within 90 minutes of `published_at`)
- Stale heartbeat (missing or older than threshold)

## Known issues

- Export always uses memory repository (Turso via HS-040 pending) ⇒ site content never changes beyond committed fixture data
- The heartbeat file commits to the repo; the refresh workflow does not push any data back to the default branch