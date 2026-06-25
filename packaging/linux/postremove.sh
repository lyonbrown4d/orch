#!/bin/sh
set -eu

is_upgrade() {
  case "${1:-}" in
    upgrade|1)
      return 0
      ;;
    *)
      return 1
      ;;
  esac
}

systemd_daemon_reload() {
  if command -v systemctl >/dev/null 2>&1 && [ -d /run/systemd/system ]; then
    if ! systemctl daemon-reload >/dev/null 2>&1; then
      echo "orch: systemd daemon-reload failed; run 'systemctl daemon-reload' manually" >&2
    fi
  fi
}

if ! is_upgrade "${1:-}"; then
  if command -v orch-server >/dev/null 2>&1; then
    if ! orch-server host-dns uninstall --non-interactive >/dev/null 2>&1; then
      echo "orch: host DNS uninstall skipped or failed; remove orch resolver config manually if needed" >&2
    fi
  fi
fi

systemd_daemon_reload
