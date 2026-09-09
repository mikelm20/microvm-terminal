#!/bin/bash
# guest-shell: login shell for the `dev` user inside each microVM.
#
# The user never reaches a bash prompt. Signals are trapped so Ctrl+C,
# Ctrl+\ and Ctrl+Z at the menu do not break out; while Claude Code is in the
# foreground it handles them itself (Ctrl+C cancels the current request).
#
# Every launch is the real interactive `claude` with permission prompts
# disabled: the VM is the permission boundary. Option 5 powers the VM off,
# which ends the session on the host side too.

set -u
trap '' INT QUIT TSTP HUP

WORKSPACE_ROOT="$HOME/workspace"
mkdir -p "$WORKSPACE_ROOT"
cd "$WORKSPACE_ROOT" || exit 1

banner() {
  clear
  printf '\n'
  printf '  \e[1mClaude Code\e[0m  in microVM \e[36m%s\e[0m\n' "$(hostname)"
  printf '  Folder: \e[36m%s\e[0m\n\n' "${PWD/#$HOME/~}"
}

menu() {
  banner
  cat <<'MENU'
  1) New session
  2) Continue the last session
  3) Pick an earlier session
  4) Change folder
  5) Exit and shut down this VM

MENU
  printf '  Choice: '
}

pause_enter() {
  printf '\n  Press Enter to return to the menu...'
  read -r _ || true
}

launch_claude() {
  claude --dangerously-skip-permissions "$@"
  local rc=$?
  if [ "$rc" -ne 0 ]; then
    printf '\n  [claude exited with status %d]\n' "$rc"
    pause_enter
  fi
}

change_dir() {
  printf '\n  Folder under ~/workspace (Enter for the root): '
  local target
  read -r target || target=""

  if [ -z "$target" ]; then
    cd "$WORKSPACE_ROOT"
    return
  fi

  # Absolute paths are taken relative to the workspace root so the user
  # cannot leave it: "/foo" becomes "$WORKSPACE_ROOT/foo".
  target="${target#/}"
  local candidate="$WORKSPACE_ROOT/$target"
  mkdir -p "$candidate" 2>/dev/null || true
  local resolved
  resolved=$(realpath -m "$candidate" 2>/dev/null || echo "")

  if [ -z "$resolved" ] || { [ "$resolved" != "$WORKSPACE_ROOT" ] && [[ "$resolved" != "$WORKSPACE_ROOT"/* ]]; }; then
    printf '\n  Path is outside ~/workspace. Ignored.\n'
    sleep 1
    return
  fi
  if [ ! -d "$resolved" ]; then
    printf '\n  Could not create that folder.\n'
    sleep 1
    return
  fi
  cd "$resolved"
}

shutdown_vm() {
  printf '\n  Shutting down. Nothing in this VM is kept.\n'
  sleep 0.5
  # sudoers grants this one command and nothing else.
  sudo -n /sbin/poweroff 2>/dev/null || exit 0
}

while :; do
  menu
  choice=""
  # read fails on EOF (Ctrl+D); treat it as a redraw, not an exit.
  if ! read -r choice; then
    sleep 0.3
    continue
  fi
  choice="${choice#"${choice%%[![:space:]]*}"}"
  choice="${choice%"${choice##*[![:space:]]}"}"

  case "$choice" in
    1)     launch_claude ;;
    2)     launch_claude --continue ;;
    3)     launch_claude --resume ;;
    4)     change_dir ;;
    5|q|Q|quit|exit)
           shutdown_vm
           ;;
    '')    ;;
    *)
           printf '\n  Not an option.\n'
           sleep 0.6
           ;;
  esac
done
