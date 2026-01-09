# Troubleshooting

## “missing .ergo/events.jsonl”

Run `ergo init` from the repo root (or the directory you want to track).

## “I ran it from the wrong directory”

Run commands from the directory that contains `.ergo/` (there’s no auto-discovery yet).

## “My editor didn’t open”

Set `EDITOR`, for example:

```sh
export EDITOR=nano
```

## “The tool feels stuck”

Writes are serialized via `.ergo/lock`. If you suspect a stale lock from a crashed process, confirm no `ergo` process is running and then remove the lock file:

```sh
rm .ergo/lock
```

