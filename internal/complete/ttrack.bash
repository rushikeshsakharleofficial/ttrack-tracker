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
        COMPREPLY=( $(compgen -W "rec play ls tail tree search export prune ansible init completion version --check help" -- "$cur") )
        return
    fi

    # Flags that take a value.
    case "$prev" in
        --speed|--idle|-n)
            return ;;  # numeric, no completion
        -o)
            COMPREPLY=( $(compgen -f -- "$cur") )
            return ;;
        --user)
            COMPREPLY=( $(compgen -W "$(ttrack __complete users 2>/dev/null)" -- "$cur") )
            return ;;
    esac

    case "$sub" in
        init)
            COMPREPLY=( $(compgen -W "--reset-password --clear-password" -- "$cur") ) ;;
        rec)
            COMPREPLY=( $(compgen -W "-q -o" -- "$cur") ) ;;
        play)
            # auto-detect: complete both local sessions and central sessions
            COMPREPLY=( $(compgen -W "--speed --idle $(ttrack __complete local-sessions 2>/dev/null) $(ttrack __complete central-sessions 2>/dev/null)" -- "$cur") ) ;;
        ls)
            COMPREPLY=( $(compgen -W "--all --user" -- "$cur") ) ;;
        tail)
            COMPREPLY=( $(compgen -W "-f -n $(ttrack __complete central-sessions 2>/dev/null)" -- "$cur") ) ;;
        export)
            COMPREPLY=( $(compgen -W "$(ttrack __complete central-sessions 2>/dev/null)" -- "$cur") ) ;;
        search)
            COMPREPLY=( $(compgen -W "--from --to --user -i --all" -- "$cur") ) ;;
        ansible)
            COMPREPLY=( $(compgen -W "list show" -- "$cur") ) ;;
        completion)
            COMPREPLY=( $(compgen -W "bash" -- "$cur") ) ;;
        *)
            COMPREPLY=() ;;
    esac
}
complete -F _ttrack ttrack
