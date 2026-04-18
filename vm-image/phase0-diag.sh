#!/bin/bash
# Phase 0 diagnostic. Runs once after multi-user.target. Dumps network + service
# state to the serial console so we can debug without interactive login.
set +e
echo
echo "===== phase0-diag begin ====="
echo "--- ip -br addr ---"
ip -br addr
echo "--- ip route ---"
ip route
echo "--- systemd-networkd status ---"
systemctl status systemd-networkd --no-pager -l 2>&1 | head -40
echo "--- networkctl ---"
networkctl status --no-pager 2>&1 | head -40
echo "--- journalctl for networkd ---"
journalctl -u systemd-networkd --no-pager -n 30 2>&1
echo "--- serial-getty override ---"
cat /etc/systemd/system/serial-getty@ttyS0.service.d/override.conf 2>&1
echo "--- resolv.conf ---"
cat /etc/resolv.conf 2>&1
echo "--- curl test ---"
timeout 5 curl -sI https://api.anthropic.com/v1/ 2>&1 | head -5
echo "===== phase0-diag end ====="
echo
