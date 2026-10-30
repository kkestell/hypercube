#!/bin/sh
# The first waiter writes a message; later ones wait until they are killed.
if [ "$1" = wait ]; then
  echo $$ >> "$HYPERCUBE_TEST_LOG.waits"
  if [ "$(wc -l < "$HYPERCUBE_TEST_LOG.waits")" -eq 1 ]; then
    printf 'Message from o (o1):\nhi\n' >&2
    exit 2
  fi
  exec sleep 30
fi
input=$(cat)
printf '%s %s\n' "$2" "$input" >> "$HYPERCUBE_TEST_LOG"
if [ "$2" = Stop ]; then
  if [ ! -f "$HYPERCUBE_TEST_LOG.stopped" ]; then
    touch "$HYPERCUBE_TEST_LOG.stopped"
    printf '%s' '{"decision":"block","reason":"queued"}'
  fi
else
  printf '{"hookSpecificOutput":{"additionalContext":"%s context"}}' "$2"
fi
