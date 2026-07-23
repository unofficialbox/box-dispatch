# Smoke test checklist

1. Confirm scenario artifacts are generated for all providers.
2. Confirm unresolved tokens are either filled or explicitly deferred.
3. Confirm local bootstrap state exists and indicates provider pass states.
4. Confirm manual task register items are clear.
5. Confirm no destructive or publish actions were performed outside `box-dispatch` confirmation gates.
