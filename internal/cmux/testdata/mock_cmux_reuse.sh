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
  ping)            echo "PONG" ;;
  list-workspaces) echo "  workspace:5  Symphony" ;;
  rename-tab)        echo "OK" ;;
  workspace-action)  echo "OK" ;;
  *)               echo "OK" ;;
esac
