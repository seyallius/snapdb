#!/bin/bash
# mysql-quickstart-entrypoint.sh - Optimized MySQL entrypoint for dbtestkit.
#
# If /var/lib/mysql is empty (fresh tmpfs), extract a pre-baked empty database
# tarball to skip the multi-second initdb first-run sequence. Then hand off to
# the official MySQL entrypoint with settings tuned for fast test execution.
set -e

if [ -z "$(ls -A /var/lib/mysql 2>/dev/null)" ] && [ -f /tmp/empty-mysql.tar.gz ]; then
  echo "==> dbtestkit: hydrating pre-baked MySQL data directory"
  tar -xzf /tmp/empty-mysql.tar.gz -C /var/lib/mysql/
  chown -R mysql:mysql /var/lib/mysql/
fi

exec docker-entrypoint.sh mysqld \
  --innodb-buffer-pool-size=16M \
  --skip-performance-schema \
  --skip-log-bin \
  --skip-mysqlx
