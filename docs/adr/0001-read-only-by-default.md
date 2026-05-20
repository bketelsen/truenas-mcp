# Read-only by default

Mutating tools (create, delete, start, stop, restart, dismiss) are not registered unless the server is started with `--enable-writes` or `TRUENAS_ENABLE_WRITES=true`. An AI client connecting to a default server cannot see or invoke them.

This fail-closed posture was chosen because first contact with a NAS appliance carries real risk — an AI client could delete datasets or stop apps before the operator has validated the tool responses. The cost of requiring an explicit opt-in is low; the cost of a destructive action on a production NAS is high. A read-only default makes the safe path the easy path.
