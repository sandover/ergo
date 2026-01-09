# FAQ

## Should I commit `.ergo/`?

By default, treat it as personal/stealth state and keep it in `.gitignore`.
If you want shared task state for a repo (especially for multiple agents across machines), you can commit `.ergo/`.

## Is it safe with multiple agents?

Yes for local concurrency: all writes are serialized via `.ergo/lock`, and `ergo take` is designed to be race-safe.

## Does it support Windows?

Not yet.

