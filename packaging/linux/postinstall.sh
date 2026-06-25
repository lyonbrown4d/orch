#!/bin/sh
set -eu

systemd_daemon_reload() {
  if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    if ! systemctl daemon-reload >/dev/null 2>&1; then
      echo "orch: systemd daemon-reload failed; run 'systemctl daemon-reload' manually" >&2
    fi
  fi
}

systemd_daemon_reload

if command -v orch-server >/dev/null 2>&1; then
  if ! orch-server host-dns install --non-interactive >/dev/null 2>&1; then
    echo "orch: host DNS install skipped or failed; run 'orch-server host-dns status' for details" >&2
  fi
fi
