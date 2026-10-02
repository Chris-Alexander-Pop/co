# co

Queue compiles on another machine, then install the results here.

`co` is a client. `co-queue` is the server, meant to run in the Arch container in `deploy/`. One job runs at a time, in the order it was submitted, so a dependency can be installed in the container before the next package builds.

If the queue cannot be reached, `fallback` in the config decides:

- `local` builds on the machine where you ran `co`
- `cancel` exits non-zero

`--fallback local` or `--fallback cancel` overrides that for one run.

## Layout

```
co ship              queue the current directory
co makepkg           build the PKGBUILD here and install the package
co aur upgrade       upgrade AUR packages through the queue
co status            list jobs
co logs [id]         print a job log
```

Config is `~/.config/co/config.yaml`. Start from `config.example.yaml`. Do not commit a file that contains a real URL or token.

`co ship` uses `makepkg` when the directory has a PKGBUILD, and `make -j` otherwise. `--no-wait` returns after the job is queued.

`co makepkg --no-install` downloads the built packages into the current directory and does not run `pacman`.

## Server

The image is Arch with `base-devel` and `git`. It refuses to start without `CO_TOKEN`.

```bash
export CO_TOKEN="$(openssl rand -hex 24)"
docker compose -f deploy/compose.yaml up -d --build
```

The example publishes `127.0.0.1:8787` only. Point that at a tailnet address if the client is another computer. Do not publish `0.0.0.0`.

Put the same URL and token in `~/.config/co/config.yaml` on the client.

## Topgrade

A custom command can call `co aur upgrade`. Pacman itself stays on the laptop. When the queue is down and `fallback` is `local`, that command builds with `yay` here instead.
