# bash completion for ttrack
# Install: ttrack completion bash | sudo tee /usr/share/bash-completion/completions/ttrack
# (packages install this automatically)

_ttrack() {
    local cur prev sub
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    sub="${COMP_WORDS[1]}"

    # Top-level subcommand.
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "rec play ls ls-user play-user tail tree search export completion help" -- "$cur") )
        return
    fi

    # Flags that take a value.
    case "$prev" in
        --speed|--idle)
            return ;;  # numeric, no completion
        -o)
            COMPREPLY=( $(compgen -f -- "$cur") )
            return ;;
    esac

    case "$sub" in
        rec)
            COMPREPLY=( $(compgen -W "-q -o" -- "$cur") ) ;;
        play)
            COMPREPLY=( $(compgen -W "--speed --idle $(ttrack __complete local-sessions 2>/dev/null)" -- "$cur") ) ;;
        ls-user)
            COMPREPLY=( $(compgen -W "$(ttrack __complete users 2>/dev/null)" -- "$cur") ) ;;
        play-user)
            COMPREPLY=( $(compgen -W "--speed --idle $(ttrack __complete central-sessions 2>/dev/null)" -- "$cur") ) ;;
        tail|export)
            COMPREPLY=( $(compgen -W "$(ttrack __complete central-sessions 2>/dev/null)" -- "$cur") ) ;;
        search)
            COMPREPLY=( $(compgen -W "--from --to --user -i" -- "$cur") ) ;;
        completion)
            COMPREPLY=( $(compgen -W "bash" -- "$cur") ) ;;
        *)
            COMPREPLY=() ;;
    esac
}
complete -F _ttrack ttrack
