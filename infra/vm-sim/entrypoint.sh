#!/bin/sh
# vm-sim/entrypoint.sh
#
# Approximates what a VM under nftables would see. Docker compose networks
# give us L2 reachability to every service; this script then drops egress
# except to the claude-proxy service and the allowlisted fake-anthropic.
#
# Not a 1:1 of infra/nftables/microvm.rules; the real rules run on the host
# and match fc-br0 packets. Here we install container-local iptables rules
# that produce the same *behaviour* from a curl's perspective.

set -eu

apk add --no-cache curl iptables bind-tools >/dev/null

# Resolve service names to IPs once; docker compose DNS is reliable.
proxy_ip="$(getent hosts claude-proxy | awk '{print $1}' | head -1 || true)"
anthropic_ip="$(getent hosts fake-anthropic | awk '{print $1}' | head -1 || true)"

if [ -z "$proxy_ip" ] || [ -z "$anthropic_ip" ]; then
  echo "vm-sim: service DNS not ready; skipping egress lockdown"
else
  # Default deny for new outbound connections; allow established replies.
  iptables -P OUTPUT ACCEPT
  iptables -F OUTPUT
  iptables -A OUTPUT -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
  iptables -A OUTPUT -o lo -j ACCEPT

  # DNS resolution toward docker's embedded resolver must stay open so
  # `curl example.com` resolves and then gets blocked at L4 rather
  # than failing earlier at DNS. Docker puts the resolver at 127.0.0.11.
  iptables -A OUTPUT -d 127.0.0.11 -p udp --dport 53 -j ACCEPT
  iptables -A OUTPUT -d 127.0.0.11 -p tcp --dport 53 -j ACCEPT

  # Allowlist: only the proxy (8443) and fake-anthropic (8080). Direct
  # anthropic should be blocked; everything else is dropped.
  iptables -A OUTPUT -d "$proxy_ip" -p tcp --dport 8443 -j ACCEPT
  iptables -A OUTPUT -d "$anthropic_ip" -p tcp --dport 8080 -j ACCEPT

  # Drop literally everything else; rate-limited LOG target keeps the
  # journal from flooding during tests.
  iptables -A OUTPUT -m limit --limit 5/s -j LOG --log-prefix "VM_SIM_DROP "
  iptables -P OUTPUT DROP
  echo "vm-sim: egress lockdown installed (proxy=$proxy_ip anthropic=$anthropic_ip)"
fi

# Hand off to whatever CMD the image was started with.
exec "$@"
