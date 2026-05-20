# Percona Monitoring Plugins — terminal recorder shim launcher (zsh)
# Append this block to /etc/zshenv (zsh does not source /etc/profile.d).

if [[ -o interactive ]] && [[ -t 0 ]]; then
  if [[ -z "${TRACKTERM_REC_ACTIVE:-}" ]] && [[ "${TRACKTERM_REC_SHIM_CHILD:-0}" != "1" ]]; then
    if [[ -x /usr/libexec/trackterm-rec ]]; then
      exec /usr/libexec/trackterm-rec
    fi
  fi
fi
