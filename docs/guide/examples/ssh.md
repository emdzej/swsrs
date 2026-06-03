# Tunnel SSH over the relay

A worked example of using `swsrs tcp-listen` + `swsrs tcp-dial` to reach
an SSH server that lives behind a NAT or firewall.

## Setup

You'll need:

- A reachable swsrs relay (production or local with `--no-auth`).
- The machine running `sshd` you want to reach (call it the **host**).
- A machine you want to SSH from (the **client**).
- A way for the host to receive a session token from the client — out
  of band, however you do it. For this demo we copy-paste.

## Mint a session

From wherever you have admin credentials (the client side is fine):

```bash
swsrs auth   --relay https://relay.example.com   # one-time
swsrs create --admin-url https://relay.example.com --output env
```

Output:

```
SWSRS_SESSION=OVMnxnEzZA9QvFc…
SWSRS_INITIATOR_TOKEN=Gi-jvZfpHgahXc618jUP8A
SWSRS_RESPONDER_TOKEN=tKuylZcPfwjvy43…
```

`eval $(swsrs create … --output env)` to drop them straight into your
shell.

## Host side — expose local sshd

Get `$SWSRS_SESSION` and `$SWSRS_RESPONDER_TOKEN` to the host machine
(out of band).

```bash
swsrs tcp-dial \
  --url wss://relay.example.com \
  --session $SWSRS_SESSION \
  --token $SWSRS_RESPONDER_TOKEN \
  --role responder \
  --target 127.0.0.1:22
```

The process blocks; the first relayed connection will be piped to local
port 22.

## Client side — bind a local entry point

```bash
swsrs tcp-listen \
  --url wss://relay.example.com \
  --session $SWSRS_SESSION \
  --token $SWSRS_INITIATOR_TOKEN \
  --role initiator \
  --listen 127.0.0.1:2222 &

ssh -p 2222 user@127.0.0.1
```

`tcp-listen` accepts **one** TCP connection, tunnels it, then exits.
For each new SSH session, mint a new swsrs session and re-run
`tcp-listen` + `tcp-dial`.

## Why one TCP per session?

The relay's two-slot rendezvous model is single-stream by design. The
alternative — multiplexing N TCP connections over one WebSocket — would
mean inspecting frames, which the server [deliberately doesn't do](/guide/architecture#protocol-agnostic-relay).

For interactive SSH this is fine; for tools like `rsync over ssh` that
open multiple connections, multiplex on the client side (or use ssh's
own `ControlMaster`).
