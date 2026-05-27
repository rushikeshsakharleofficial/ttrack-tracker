# ttrack Ansible callback plugin

Records Ansible playbook runs into the ttrack central store.

## Install

The plugin ships at `/usr/share/ttrack/ansible/ttrack.py` (installed by the deb/rpm).

## Enable

**Via environment variables (per-run):**
```bash
export ANSIBLE_CALLBACK_PLUGINS=/usr/share/ttrack/ansible
export ANSIBLE_CALLBACKS_ENABLED=ttrack
ansible-playbook site.yml
```

**Via ansible.cfg (persistent):**
```ini
[defaults]
callback_plugins   = /usr/share/ttrack/ansible
callbacks_enabled  = ttrack
```

## View runs

```bash
sudo ttrack ansible list
sudo ttrack ansible show <runid>
```

## Requirements

- `ttrack` binary on `PATH` on the **controller** host
- `ttrackd` daemon running (or fail-open: saves to `~/.local/share/ttrack/ansible/`)
- Python 3.6+ (Ansible's own interpreter)

## Fail-open

If `ttrackd` is unreachable the run is saved to
`~/.local/share/ttrack/ansible/<runid>.ajsonl` (unencrypted, owner-only `0600`).
The playbook run is never aborted due to ttrack failures.
