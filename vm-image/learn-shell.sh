#!/bin/bash
# learn-shell: login shell for the `learner` user inside Firecracker guests.
#
# This wrapper exists for one reason: containment. The lesson UI assumes the
# user is always inside Claude Code, never at a bash prompt. If a user could
# drop to bash they could break the lesson flow, escalate via setuid binaries
# we forgot about, or just confuse themselves.
#
# Behavior:
#   - SIGINT / SIGQUIT / SIGTSTP / SIGHUP are trapped at the wrapper level so
#     Ctrl+C / Ctrl+\ / Ctrl+Z / disconnect don't drop the user to bash.
#     While Claude Code is in the foreground, those signals reach Claude (it
#     traps Ctrl+C itself for "interrupt the request"). When Claude exits and
#     control returns to this script, the trap absorbs them.
#   - Ctrl+D and `/exit` and Claude crashes all just cause Claude to exit;
#     the loop relaunches it immediately, keeping the user in Claude.
#   - --dangerously-skip-permissions: lessons must not block on tool-use
#     prompts or per-folder trust dialogs. The VM is ephemeral and isolated
#     by Firecracker, so the prompt-skip is safe in this sandbox.

set -u

trap '' INT QUIT TSTP HUP

# Default lesson cwd. Per-lesson overrides arrive via the kernel cmdline
# parameter `learn.cwd=<absolute-path>`, which the control plane sets when
# spawning the VM. Fallback: empresa-prueba root.
DEFAULT_CWD=/home/learner/empresa-prueba
LEARN_CWD=$(awk '
  {
    for (i = 1; i <= NF; i++) {
      if ($i ~ /^learn\.cwd=/) {
        sub(/^learn\.cwd=/, "", $i);
        print $i;
        exit;
      }
    }
  }
' /proc/cmdline 2>/dev/null)
if [[ -z "${LEARN_CWD}" ]]; then
  LEARN_CWD="${DEFAULT_CWD}"
fi
mkdir -p "${LEARN_CWD}" 2>/dev/null || true

cd "${LEARN_CWD}" 2>/dev/null || cd "${HOME}"

mkdir -p /run/learn 2>/dev/null || true

while :; do
  # claude-wrap spawns `claude --dangerously-skip-permissions
  # --output-format stream-json --input-format stream-json --verbose` and
  # emits canonical WsEvents on /run/learn/claude-wrap.sock for the
  # guest-agent to relay over vsock. The learner still sees a chat on this
  # PTY because claude-wrap echoes assistant text and tool-call hints.
  /usr/local/bin/claude-wrap \
    --socket /run/learn/claude-wrap.sock \
    --cwd "${LEARN_CWD}" \
    2>>/var/log/claude-wrap.log || true
  # If we get here, claude-wrap exited. Don't surface it to the user (no
  # bash prompt), just relaunch. A tiny sleep avoids a fork bomb if the
  # wrapper crashes instantly for some reason.
  sleep 0.5
done
