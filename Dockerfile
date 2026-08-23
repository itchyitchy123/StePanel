FROM golang:1.23-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Commit=docker -X main.BuildDate=container" -o /out/stepanel .

FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates mariadb-client \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --home-dir /opt/stepanel --shell /usr/sbin/nologin stepanel \
    && mkdir -p /opt/stepanel/web/static /var/lib/ste-panel/imports /var/www/sites \
    && chown -R stepanel:stepanel /opt/stepanel /var/lib/ste-panel /var/www/sites
COPY --from=build /out/stepanel /opt/stepanel/stepanel
COPY web/index.html /opt/stepanel/web/index.html
COPY web/static/ /opt/stepanel/web/static/
USER stepanel
WORKDIR /opt/stepanel
ENV STEPANEL_LISTEN=:8080 STEPANEL_IMPORT_ROOT=/var/lib/ste-panel/imports STEPANEL_WEB_ROOT=/var/www
EXPOSE 8080
ENTRYPOINT ["/opt/stepanel/stepanel"]
