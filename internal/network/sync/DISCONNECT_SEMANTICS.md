# Disconnect and Reconnect Semantics in Peer Sync

The motor-autonomo peer sync subsystem relies heavily on durable cursors and deterministic message generation to survive disconnects and crashes.

## Durable Cursors
Each stream maintains two directional cursors:
- Outbound: Marks the highest sequence successfully processed by the remote peer. Advanced only upon receiving a valid ACK.
- Inbound: Marks the highest sequence successfully committed locally from the remote peer. Advanced when an EVENT_BATCH is stored.

## Disconnect Scenarios

### 1. Request lost before reaching remote
- Local node sends PULL.
- Network disconnects. Remote node never sees the PULL.
- Local node hits a timeout or connection reset. PullOnce returns an error.
- The local cursor is unmodified. The operation is retried later and succeeds cleanly.

### 2. Response lost before reaching local
- Remote node receives PULL, builds an EVENT_BATCH, sends it.
- Network disconnects. Local node never receives the EVENT_BATCH.
- Remote node has no state to roll back (it was a pure read).
- Local node PullOnce returns an error. Cursor unmodified. Retried later. Remote node generates the exact same batch again.

### 3. Crash before local commit
- Local node receives EVENT_BATCH.
- Node crashes right before or during Store.Update.
- Cursor remains unmodified. Retried later.

### 4. Crash or disconnect after local commit, before ACK
- Local node receives EVENT_BATCH.
- Node commits events and advances Inbound cursor in the local store.
- Local node crashes OR network disconnects before the ACK can be sent over the RPC.
- Remote node never receives the ACK. Remote node outbound cursor remains unadvanced.
- When the local node restarts/reconnects and issues PullOnce, the code checks inboundPosition.
- Finding an existing inbound cursor that has not necessarily been ACKed, it pre-emptively sends an ACK for after before issuing the next PULL.
- This allows the remote node to finally advance its Outbound cursor and prepare the next batch.

## Idempotency
- All MessageID and RequestID fields are generated deterministically.
- If an ACK is sent twice, it has the exact same MessageID.
- The protocol mandates that repeating an already-known cursor is a no-op, not an error.

