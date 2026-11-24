# Retention packages (issue #120 / HS-065)
#
# This directory holds portable retention packages exported before hot-table
# archival. Each package is a .tar.gz in huddlescope-retention-package/v1
# format: manifest.json + tables/{articles,stories,ranking_snapshots}.sql.
# Restore with: ./dist/huddlescope retention import --package <file>.tar.gz
