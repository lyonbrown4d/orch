FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY orch-server /usr/local/bin/orch-server

EXPOSE 17443 7444 18080
ENTRYPOINT ["/usr/local/bin/orch-server"]
