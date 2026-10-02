FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /co-queue ./cmd/co-queue

FROM archlinux:base
RUN pacman -Syu --noconfirm \
	&& pacman -S --noconfirm base-devel git sudo ccache bc bison flex cpio perl python pahole elfutils kmod openssl rsync \
	&& pacman -Scc --noconfirm
RUN useradd -m builder \
	&& printf 'builder ALL=(ALL) NOPASSWD: ALL\n' >> /etc/sudoers
COPY deploy/makepkg.co.conf /etc/makepkg.conf.d/co.conf
COPY deploy/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
COPY --from=build /co-queue /usr/local/bin/co-queue
ENV CO_ROOT=/var/lib/co \
	CCACHE_DIR=/var/cache/ccache \
	CCACHE_MAXSIZE=20G \
	PATH="/usr/lib/ccache/bin:${PATH}"
EXPOSE 8787
ENTRYPOINT ["/entrypoint.sh"]
