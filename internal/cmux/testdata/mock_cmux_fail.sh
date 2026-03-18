#!/bin/bash
# Filter out --id-format and its value for logging
ARGS=()
SKIP_NEXT=false
for arg in "$@"; do
  if $SKIP_NEXT; then SKIP_NEXT=false; continue; fi
  if [ "$arg" = "--id-format" ]; then SKIP_NEXT=true; continue; fi
  ARGS+=("$arg")
done
echo "${ARGS[@]}" >> "${CMUX_TEST_LOG}"

CMD="${ARGS[0]}"
case "$CMD" in
  ping) echo "ERROR: socket not found" >&2; exit 1 ;;
  *)    echo "OK" ;;
esac
