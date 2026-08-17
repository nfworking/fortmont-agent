# Fortmont Agent

The Fortmont Agent is the node-side runtime for Fortmont. It is deliberately responsible only for the agent side of the control-plane/WebSocket contract; durable agent authorization remains in Fortmont Control Plane and realtime presence remains in Redis through the Fortmont WS gateway.

## Lifecycle

### First installation

1. The Control Plane creates a pending agent and one-time enrollment token.
2. The agent starts with `--token <enrollment-token>` (or `FORTMONT_ENROLLMENT_TOKEN`).
3. On first startup the agent generates an Ed25519 keypair locally and persists the private key with restrictive file permissions.
4. The agent generates a stable device ID and collects hostname/platform/architecture/version/local IP metadata.
5. Every configured WebSocket gateway is probed concurrently. The lowest-latency reachable gateway is selected.
6. The agent connects to `<selected>/ws` and sends `register` with the enrollment token and public key.
7. The WS gateway forwards registration to the authenticated Control Plane internal API.
8. After `registration_complete`, the agent persists `agent_id` and `key_id` but never persists the enrollment token.
9. The gateway sends a fresh challenge. The agent signs the challenge with its Ed25519 private key.
10. After `authenticated`, the agent sends heartbeats and remains connected.

### Subsequent startup/reconnect

Once `agent_id` and `key_id` exist, the enrollment token is no longer required. The agent probes the configured gateway nodes, selects the fastest reachable node, sends `authenticate`, signs a fresh challenge and resumes heartbeat/ping traffic.

If the connection fails, the agent retries with exponential backoff capped by `FORTMONT_RECONNECT_MAX_SEC` and selects the fastest reachable gateway again. This allows multiple WS nodes behind a load balancer or DNS-independent node list.

If the Control Plane revokes the agent, the gateway closes the connection through the Redis revocation event path. A subsequent authentication is rejected by the Control Plane even though the agent still possesses its private key.

## Gateway selection

Configure multiple nodes as a comma-separated list:

```env
FORTMONT_WS_NODES=wss://agents-1.fortmont.me/ws,wss://agents-2.fortmont.me/ws,wss://agents-3.fortmont.me/ws
```

At startup the agent measures TCP reachability (and the TLS handshake for `wss`) for every node concurrently. The node with the lowest successful connection latency is selected. The actual WebSocket handshake is then performed against that URL.

## Configuration

Copy `.env.example` to `.env` for local development. `.env` is ignored by Git. Process environment variables take precedence over `.env` values.

Important variables:

- `FORTMONT_WS_NODES` — required, comma-separated `ws://`/`wss://` gateway URLs.
- `FORTMONT_ENROLLMENT_TOKEN` — optional bootstrap token; `--token` is preferred for first enrollment.
- `FORTMONT_CONFIG_DIR` — credential directory. Defaults to the OS user config directory.
- `FORTMONT_VERSION` — agent version reported to Fortmont.
- `FORTMONT_PUBLIC_IP` — optional externally observed public IP. Left blank if not configured.
- `FORTMONT_CONNECT_TIMEOUT_SEC` — gateway connection/probe timeout.
- `FORTMONT_RECONNECT_MAX_SEC` — reconnect backoff ceiling.
- `FORTMONT_HEARTBEAT_SEC` — heartbeat interval.
- `FORTMONT_PING_INTERVAL_SEC` — WebSocket ping interval.

## Local development

```powershell
Copy-Item .env.example .env
go mod tidy
go run ./cmd/agent --token ft_enroll_...
```

After successful enrollment:

```powershell
go run ./cmd/agent
```

The persisted credential file contains the Ed25519 private key and is protected with mode `0600` on platforms that support Unix-style permissions. It is never sent to the gateway or Control Plane.

## Protocol contract

The agent implements the current `fortmont-ws` contract:

```text
register
  -> registration_complete
  -> challenge
  -> challenge_response
  -> authenticated
  -> heartbeat / ping
```

and for an enrolled identity:

```text
authenticate
  -> challenge
  -> challenge_response
  -> authenticated
  -> heartbeat / ping
```

Messages use the same envelope as the gateway:

```json
{"type":"heartbeat","data":{}}
```

The agent does not know or use `CONTROL_PLANE_INTERNAL_SECRET`. That secret is strictly gateway-to-Control-Plane service authentication and must never be distributed to an installed agent.

## Security properties

- Enrollment token is a one-time bootstrap credential and is not stored after registration.
- Ed25519 private key remains on the node.
- Public key is the durable agent identity presented during enrollment.
- Every connection requires a fresh challenge signature.
- Agent identity and authorization are distinct: possession of the private key does not override Control Plane revocation.
- Credential files are created in a restricted directory and written atomically.
- Enrollment tokens and private keys are not intentionally logged.
