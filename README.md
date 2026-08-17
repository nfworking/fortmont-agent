# Fortmont Agent

The Fortmont Agent is the node-side runtime for Fortmont. It implements the agent side of the Control Plane/WebSocket contract; durable authorization remains in Fortmont Control Plane and realtime presence remains in Redis through the Fortmont WS gateway.

## Lifecycle

### First installation

1. The Control Plane creates a pending agent and one-time enrollment token.
2. The agent starts with `--token <enrollment-token>` or is installed as a service with `service install --token <enrollment-token>`.
3. On first startup the agent generates an Ed25519 keypair locally and persists the private key with restrictive file permissions.
4. The agent generates a stable device ID and collects hostname/platform/architecture/version/local IP metadata.
5. Every configured WebSocket gateway is probed concurrently. The lowest-latency reachable gateway is selected.
6. The agent connects to the selected gateway and sends `register` with the enrollment token and public key.
7. The WS gateway forwards registration to the authenticated Control Plane internal API.
8. After `registration_complete`, the agent persists `agent_id` and `key_id` and deletes the local bootstrap token file.
9. The gateway sends a fresh challenge. The agent signs the challenge with its Ed25519 private key.
10. After `authenticated`, the agent sends heartbeats and remains connected.

### Subsequent startup/reconnect

Once `agent_id` and `key_id` exist, the enrollment token is no longer required. The agent probes the configured gateway nodes, selects the fastest reachable node, sends `authenticate`, signs a fresh challenge and resumes heartbeat/ping traffic.

If the connection fails, the agent retries with exponential backoff capped by `FORTMONT_RECONNECT_MAX_SEC` and selects the fastest reachable gateway again. This supports multiple WS nodes without putting any gateway-specific identity state on the agent.

If the Control Plane revokes the agent, the gateway closes the connection through the Redis revocation event path. A subsequent authentication is rejected by the Control Plane even though the agent still possesses its private key.

## Service management

The agent supports native service management on Windows and Linux:

```text
fortmont-agent service install --token <enrollment-token>
fortmont-agent service status
fortmont-agent service uninstall
```

`service install` installs and starts the agent immediately. On Windows it creates the `FortmontAgent` Windows service. On Linux it creates a systemd unit named `fortmont-agent.service`.

The service installer requires `FORTMONT_WS_NODES` to be present in the environment at installation time. Runtime configuration is copied into a protected `service.env` file so the service does not depend on the installer's current working directory or `.env` file. The one-time enrollment token is stored separately with restricted permissions and is deleted after successful registration.

Service installation should be run from an elevated Administrator shell on Windows or as root on Linux.

The service command is intentionally separate from normal foreground execution:

```text
fortmont-agent --token <enrollment-token>
fortmont-agent
fortmont-agent service install --token <enrollment-token>
```

This makes the binary usable both interactively during development and as a long-running operating-system service in production.

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
- `FORTMONT_CONFIG_DIR` — credential directory. Defaults to the OS user config directory for foreground execution. Managed services use their protected service directory.
- `FORTMONT_VERSION` — agent version reported to Fortmont.
- `FORTMONT_PUBLIC_IP` — optional externally observed public IP. Left blank if not configured.
- `FORTMONT_CONNECT_TIMEOUT_SEC` — gateway connection/probe timeout.
- `FORTMONT_RECONNECT_MAX_SEC` — reconnect backoff ceiling.
- `FORTMONT_HEARTBEAT_SEC` — heartbeat interval.
- `FORTMONT_PING_INTERVAL_SEC` — WebSocket ping interval.

The enrollment token is intentionally not a permanent credential. Supply it directly during the first installation:

```powershell
go run ./cmd/agent --token ft_enroll_...
```

For service installation:

```powershell
go run ./cmd/agent service install --token ft_enroll_...
```

After successful enrollment, the token is removed and future starts use the persisted Ed25519 identity.

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

- Enrollment token is a one-time bootstrap credential and is deleted after successful enrollment.
- Ed25519 private key remains on the node.
- Public key is the durable agent identity presented during enrollment.
- Every connection requires a fresh challenge signature.
- Agent identity and authorization are distinct: possession of the private key does not override Control Plane revocation.
- Credential and bootstrap files are created in a restricted directory.
- Enrollment tokens and private keys are not intentionally logged.
