# phase-0-spike

Throwaway scripts that prove the Firecracker + Claude Code primitives work end to end before any control-plane code is written.

## Usage (on learn-01)

```
cd ~/learn-platform/phase-0-spike
sudo bash spike-run.sh
```

You will see the kernel boot messages and land at a shell logged in as `learner`. To test:

```
# inside the VM
ip addr show eth0          # should show 172.20.0.2/24
curl -sI https://api.anthropic.com    # confirm egress
export CLAUDE_CODE_OAUTH_TOKEN=<token from `claude setup-token`>
claude --print "hello"
```

When done, Ctrl+C to kill Firecracker. Then optionally:

```
sudo bash teardown-network.sh
sudo rm -rf /var/lib/firecracker/vms/spike
```

## Scope

This spike intentionally does NOT include:
- Jailer (Phase 1 wraps Firecracker with jailer)
- vsock guest agent (Phase 2)
- Egress lockdown (Phase 2 restricts to api.anthropic.com + apt)
- Copy-on-write overlays (Phase 1 uses backing-file overlays per VM)

The spike's only purpose is to answer: can we boot a Firecracker guest, reach the network, and run Claude Code inside it?
