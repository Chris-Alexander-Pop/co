# compile-offload

Queue compiles on another machine, then install the results here. The command is `co`.

`co --command cargo build --release` runs that command on the builder. The current directory is uploaded as the source tree. `target`, `node_modules`, and `.cache` are left out. The builder installs missing tools, including a Rust toolchain with the wasm32 target when the command uses cargo. New binaries from `cargo install` or `go install` are copied into `~/.cargo/bin` here. Executables written to `target/release` or `target/debug` are copied back into this tree.

`--bg` and `--fallback` go before `--command`:

```
co --bg --command cargo test
```

`--seed-cache` builds on this machine instead of the queue, uploads ccache plus the Go and Cargo caches to the builder, then deletes the local seed under `~/.cache/co/seed`. Later remote jobs can reuse that warm cache. The builder must be reachable. It cannot be combined with `--bg`. Cache hits need compilers and toolchains close to the Arch builder's.

```
co --seed-cache --command cargo build --release
```

`co` is a client. `co-queue` is the server, meant to run in the Arch container in `deploy/`. It fills the builder's CPUs. A strategy build uses the `jobs` count from the client, and two of those run side by side when the threads fit. `co --command` and package builds (`makepkg`, AUR) take the whole machine one at a time. They install packages into the same container, and command jobs share one cargo home.

Submitting the same tree again does not queue a second job and does not unpack over the one already there. The call returns the job that is queued, running, or finished with that same source, build commands, and pull list. A failed job can be submitted again. An AUR package that is already queued or running is not submitted twice. A later upgrade of that package still queues, because the recipe is fetched when the build starts.

If the queue cannot be reached, `fallback` in the config decides:

- `local` builds on the machine where you ran `co`
- `cancel` exits non-zero

`--fallback local` or `--fallback cancel` overrides that for one run.

## Layout

```
co --command <command...>   run that command on the builder and bring binaries back
co                   build this directory from its strategy, or its PKGBUILD
co --bg              queue the build and return
co --seed-cache      build here, upload ccache/Go/Cargo caches, prune local seed
co status            live board: cores, running builds, queue, ETA
co finish            install the newest finished build into its project directory
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
  - sudo rsync -a --chown=root:root out/lib/modules/ /lib/modules/
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
- the Rust toolchain under `/opt/rustup`, and the cargo registry and git checkouts
- the Go build cache
- source tarballs makepkg already downloaded
- pacman's downloaded packages
- packages installed into the container by earlier jobs

`co --seed-cache` can fill ccache, the Go build cache, and Cargo's registry/git from a local build (`POST /v1/cache`), then remove the laptop seed directory.

`jobs` in the client config is the `make -j` count for a strategy or a tree you upload. Keep it at or under `CO_CPUS` on the server. The server clamps to that budget and exports `MAKEFLAGS`, `NINJAFLAGS`, and `CMAKE_BUILD_PARALLEL_LEVEL` to match. Set `CO_CPUS` and `CO_CPU_LIMIT` in `deploy/.env` when the container should leave cores free for other work. That file is gitignored. `co status` draws those cores, the jobs still waiting, and an ETA. The ETA starts from the last successful build of that project (how long it took, and its files per second) and then scales by this run's own average speed. The first build of a project has no last run, so that one uses this session's average against the meter's remaining files, and it becomes the baseline for the next one. `GET /v1/host` reports `cpus`, `busy`, `running`, and `queued`.

## Topgrade

A custom command can call `co aur upgrade`. Pacman itself stays on the laptop. When the queue is down and `fallback` is `local`, that command builds with `yay` here instead.

Each package base is built, pulled back, and installed here before the next one starts. Split packages share one repo: `gcc14-libs` is cloned as `gcc14`. AUR dependencies that pacman cannot install, such as `unionfs-fuse` or `libudev0-shim`, are built and installed first. If the builder cannot install the result, the package file is still pulled and installed here.

If a package fails and there is no package file, co asks whether to skip it and continue. Packages already installed stay installed.
