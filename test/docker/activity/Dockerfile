# Linux X11 end-to-end test for `keepalive --active`: Xvfb + xdotool +
# xprintidle. Build from the repository root:
#
#   docker build -f test/docker/activity/Dockerfile -t keepalive-activity-e2e .
#   docker run --rm keepalive-activity-e2e
FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -o /out/keepalive ./cmd/keepalive

FROM ubuntu:24.04
RUN apt-get update \
	&& DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
		xvfb xdotool xprintidle x11-xserver-utils jq \
	&& rm -rf /var/lib/apt/lists/*
COPY --from=build /out/keepalive /usr/local/bin/keepalive
COPY test/docker/activity/run.sh /usr/local/bin/run-activity-e2e
ENTRYPOINT ["/usr/local/bin/run-activity-e2e"]
