FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /co-queue ./cmd/co-queue

FROM archlinux:base
RUN pacman -Syu --noconfirm \
	&& pacman -S --noconfirm base-devel git sudo \
	&& pacman -Scc --noconfirm
RUN useradd -m builder \
	&& printf 'builder ALL=(ALL) NOPASSWD: ALL\n' >> /etc/sudoers
COPY --from=build /co-queue /usr/local/bin/co-queue
ENV CO_ROOT=/var/lib/co
EXPOSE 8787
ENTRYPOINT ["/usr/local/bin/co-queue"]
