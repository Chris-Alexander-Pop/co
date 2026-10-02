# co

Queue compiles on another machine, then install the results here.

`co` is a client. `co-queue` is the server, meant to run in the Arch container in `deploy/`. It fills the builder's CPUs. A strategy build uses the `jobs` count from the client, and two of those run side by side when the threads fit. Package builds (`makepkg`, AUR) take the whole machine one at a time, because they install into the same container.

If the queue cannot be reached, `fallback` in the config decides:

- `local` builds on the machine where you ran `co`
- `cancel` exits non-zero

`--fallback local` or `--fallback cancel` overrides that for one run.

## Layout

```
co                   build this directory from its strategy, or its PKGBUILD
co --bg              queue the build and return
co status            live board: cores, running builds, queue, ETA
co finish            pull the finished build and install it
co makepkg           build the PKGBUILD here and install the package
co aur upgrade       upgrade AUR packages through the queue
co logs [id]         print the raw build log
```

Config is `~/.config/co/config.yaml`. Start from `config.example.yaml`. Do not commit a file that contains a real URL or token.

Run `co` in a project directory. If that directory has a PKGBUILD and no strategy file, `co` does what `co makepkg` does. Otherwise it looks for a strategy under `~/.config/co/projects/`. Each file names the directory it belongs to. Build commands run on the builder. The paths under `pull` are copied back into the project directory, and then the install commands run here.

`{jobs}` becomes the `jobs` value from the client config. `{kernelrelease}` becomes the output of `make kernelrelease` when the tree has a kernel Makefile. `skip` names are matched against each file and directory name so a kernel tree does not upload its object files.

```yaml
dir: /path/to/zen-kernel
skip:
  - "*.o"
  - out
build:
  - make -j{jobs} bzImage modules
  - make modules_install INSTALL_MOD_PATH=out
  - install -D arch/x86/boot/bzImage out/boot/vmlinuz
pull:
  - out
install:
  - sudo rsync -a out/lib/modules/ /lib/modules/
  - sudo install -D out/boot/vmlinuz /boot/vmlinuz-linux-zen-{kernelrelease}
  - sudo mkinitcpio -k {kernelrelease} -g /boot/initramfs-linux-zen-{kernelrelease}.img
  - sudo grub-mkconfig -o /boot/grub/grub.cfg
```

`co makepkg --no-install` downloads the built packages into the current directory and does not run `pacman`. `--no-wait` on `co` returns after the job is queued and does not install.

## Server

The image is Arch with `base-devel` and `git`. It refuses to start without `CO_TOKEN`.

```bash
export CO_TOKEN="$(openssl rand -hex 24)"
docker compose -f deploy/compose.yaml up -d --build
```

The example publishes `127.0.0.1:8787` only. Point that at a tailnet address if the client is another computer. Do not publish `0.0.0.0`.

Put the same URL and token in `~/.config/co/config.yaml` on the client.

## What stays on the builder

The build directory is deleted when a job finishes. The log and the built packages from the last 8 jobs stay, so a download can be retried.

These caches stay on purpose. They are what make the next build faster:

- ccache, capped at 20 GB
- source tarballs makepkg already downloaded
- pacman's downloaded packages
- packages installed into the container by earlier jobs

`jobs` in the client config is the `make -j` count for a strategy or a tree you upload. Keep it at or under `CO_CPUS` on the server. The server clamps to that budget and exports `MAKEFLAGS`, `NINJAFLAGS`, and `CMAKE_BUILD_PARALLEL_LEVEL` to match. Set `CO_CPUS` and `CO_CPU_LIMIT` in `deploy/.env` when the container should leave cores free for other work. That file is gitignored. `co status` draws those cores, the jobs still waiting, and an ETA. The ETA starts from the last successful build of that project (how long it took, and its files per second) and then scales by this run's own average speed. The first build of a project has no last run, so that one uses this session's average against the meter's remaining files, and it becomes the baseline for the next one. `GET /v1/host` reports `cpus`, `busy`, `running`, and `queued`.

## Topgrade

A custom command can call `co aur upgrade`. Pacman itself stays on the laptop. When the queue is down and `fallback` is `local`, that command builds with `yay` here instead.
