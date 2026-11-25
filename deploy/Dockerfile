FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /co-queue ./cmd/co-queue

FROM archlinux:base
# lib32-* build dependencies live in multilib. core has lib32-glibc, not lib32-alsa-lib.
RUN printf '\n[multilib]\nInclude = /etc/pacman.d/mirrorlist\n' >> /etc/pacman.conf \
	&& pacman -Syu --noconfirm \
	&& pacman -S --noconfirm base-devel git sudo ccache bc bison flex cpio perl python pahole elfutils kmod openssl rsync \
		rustup nasm binaryen protobuf pkgconf clang cmake go \
	&& pacman -Scc --noconfirm
RUN useradd -m builder \
	&& printf 'builder ALL=(ALL) NOPASSWD: ALL\n' >> /etc/sudoers
COPY deploy/makepkg.co.conf /etc/makepkg.conf.d/co.conf
COPY deploy/makepkg-with-keys /opt/co/makepkg-with-keys
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
COPY --from=build /co-queue /usr/local/bin/co-queue
ENV CO_ROOT=/var/lib/co \
	CCACHE_DIR=/var/cache/ccache \
	CCACHE_MAXSIZE=20G \
	RUSTUP_HOME=/opt/rustup \
	CARGO_HOME=/opt/cargo \
	PATH="/opt/cargo/bin:/usr/lib/ccache/bin:${PATH}"
RUN rustup default stable \
	&& rustup target add wasm32-unknown-unknown
EXPOSE 8787
ENTRYPOINT ["/entrypoint.sh"]
