# Retention packages (issue #120 / HS-065)
#
# Local-development convenience ONLY (issue #252 / PUB-06): this directory is
# where `huddlescope retention export --output archive/retention` writes
# packages during local runs of `make retention` / `huddlescope retention run`.
#
# CI no longer writes or commits here. In production, retention/archival runs
# in the dedicated `retention.yml` workflow, which persists pre-delete
# packages to the private archive repository
# (HuddleScope/huddlescope-retention-archive) — durable, PAT-restricted
# storage outside this repository's Git history. Generated packages under this
# directory must never be committed (see docs/ops-runbook.md, "Retention and
# archiving").
#
# Each package is a .tar.gz in huddlescope-retention-package/v1 format:
# manifest.json + tables/{articles,stories,ranking_snapshots}.sql.
# Restore with: ./dist/huddlescope retention import --package <file>.tar.gz
