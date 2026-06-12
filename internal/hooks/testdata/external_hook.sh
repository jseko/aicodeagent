#!/bin/sh
mode="$1"
case "$mode" in
  allow)
    printf '{"decision":"allow"}'
    ;;
  deny)
    printf '{"decision":"deny","reason":"json blocked"}'
    ;;
  update)
    printf '{"decision":"allow","updated_input":{"file_path":"safe.go"}}'
    ;;
  invalid)
    printf 'debug noise'
    ;;
  exit2)
    printf 'exit blocked' >&2
    exit 2
    ;;
  halt)
    printf 'halt turn' >&2
    exit 49
    ;;
  fail)
    printf 'bad exit' >&2
    exit 7
    ;;
  sleep)
    sleep 2
    printf '{"decision":"allow"}'
    ;;
  claude)
    printf '{"hookSpecificOutput":{"permissionDecision":"deny","permissionDecisionReason":"claude blocked"}}'
    ;;
  *)
    printf '{"decision":"none"}'
    ;;
esac
