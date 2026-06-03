# CLI

The `swsrs` binary is one entry point with several subcommands. Run
`swsrs <command> --help` for command-specific flags.

```
swsrs serve           run the relay server
swsrs auth            log in via OIDC device flow (saves credentials.json)
swsrs create          create a session (uses the admin API)
swsrs tcp-listen      accept local TCP and tunnel through the relay
swsrs tcp-dial        receive a relayed connection and dial a local TCP target
swsrs raw             bridge stdin/stdout to a relay session
swsrs version         print build information
```

## serve

Runs the relay. See the [Configuration reference](/reference/configuration)
for every flag and env var.

```bash
swsrs serve --oidc-issuer https://idp/realms/foo --oidc-audience swsrs
```

## auth

Drives OAuth 2.0 device authorization against the IdP the relay
publishes via `/.well-known/swsrs-config`. Caches the resulting token to
the OS user-config directory.

```bash
swsrs auth --relay https://relay.example.com
swsrs auth --logout                 # clear the cached token
```

| Flag | Purpose |
|---|---|
| `--relay <url>`        | Relay base URL (or `SWSRS_URL`) |
| `--client-id <id>`     | OAuth `client_id` to use; defaults to discovery's `client_id_hint` |
| `--credentials <path>` | Override credentials file location |
| `--logout`             | Remove cached credentials and exit |

## create

Calls the admin API to mint a session. Reads the credentials cache if
`--oidc-token` isn't supplied.

```bash
swsrs create --admin-url https://relay.example.com
swsrs create --admin-url … --output env > session.env
```

| Flag | Purpose |
|---|---|
| `--admin-url <url>`    | Admin API base URL (or `SWSRS_ADMIN_URL`) |
| `--oidc-token <jwt>`   | Bearer token; overrides credentials cache |
| `--credentials <path>` | Override credentials file location |
| `--output json\|env`   | Output format; `env` emits eval-able shell vars |

## tcp-listen / tcp-dial

Tunnel a single TCP connection through the relay. Pair them: one side
listens locally, the other dials a local target.

```bash
# host side (exposes its SSH server)
swsrs tcp-dial   --url $RELAY --session $ID --token $RESP --role responder \
                 --target 127.0.0.1:22

# client side (gets a local entrypoint)
swsrs tcp-listen --url $RELAY --session $ID --token $INIT --role initiator \
                 --listen 127.0.0.1:2222
ssh -p 2222 user@127.0.0.1
```

One TCP connection per session. For N concurrent tunnels, mint N
sessions.

## raw

Bridge stdin/stdout to a relay session. Useful for scripting and
debugging.

```bash
swsrs raw --url $RELAY --session $ID --token $TOK --role initiator <input.bin >output.bin
```

## version

Prints build info — version, commit, build date — populated by
GoReleaser at release time. Local `go build` leaves them as `dev /
unknown / unknown`.

```bash
swsrs version
# swsrs 0.2.1
# commit:  abc1234
# built:   2026-06-03T12:00:00Z
```
