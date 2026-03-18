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

# Count how many times list-workspaces has been called
LIST_COUNT=$(grep -c "list-workspaces" "${CMUX_TEST_LOG}" 2>/dev/null || echo 0)

case "$CMD" in
  ping)              echo "PONG" ;;
  version)           echo "cmux 0.61.0 (73)" ;;
  new-workspace)     echo "OK 12345678-ABCD-1234-ABCD-123456789ABC" ;;
  list-workspaces)
    if [ "$LIST_COUNT" -le 1 ]; then
      echo "  workspace:1  Terminal 1"
    else
      printf "  workspace:1  Terminal 1\n  workspace:2  Terminal 2"
    fi
    ;;
  rename-tab)        echo "OK" ;;
  workspace-action)  echo "OK" ;;
  new-surface)       echo "OK surface:1 pane:1 workspace:1" ;;
  close-surface)     echo "OK" ;;
  surface.send_text) echo "OK" ;;
  *)                 echo "OK" ;;
esac
