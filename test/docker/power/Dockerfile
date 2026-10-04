# Linux sleep-inhibitor test against python-dbusmock: a private system and
# session bus with mocked logind, org.freedesktop.ScreenSaver,
# org.gnome.SessionManager and org.freedesktop.PowerManagement.
# Run test/docker/power/run.sh from anywhere; the build context is the
# repository root.
FROM golang:1.25 AS go

FROM ubuntu:24.04
RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      ca-certificates dbus python3-dbusmock systemd \
 && rm -rf /var/lib/apt/lists/*
COPY --from=go /usr/local/go /usr/local/go
ENV PATH=/usr/local/go/bin:$PATH CGO_ENABLED=0 GOFLAGS=-buildvcs=false
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go test -c -o /dev/null ./internal/power/
CMD ["python3", "test/docker/power/mock.py"]
