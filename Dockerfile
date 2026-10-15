# Small image: just the one program, no operating system inside.
FROM --platform=$BUILDPLATFORM golang:1.24 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/contest-dashboard .

FROM scratch
COPY --from=build /out/contest-dashboard /contest-dashboard
# Certificates so the map tiles can be fetched over HTTPS.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Settings and the contact log live in /data; keep it on a volume so they survive updates.
# Settings can be changed from any computer that can reach the page, since you can't sit
# "at" a container.
ENV CONTEST_DASHBOARD_DATA=/data CONTEST_DASHBOARD_REMOTE_SETTINGS=1
VOLUME /data
EXPOSE 8060/tcp 12060/udp 12061/udp 12062/udp
ENTRYPOINT ["/contest-dashboard", "-no-browser"]
