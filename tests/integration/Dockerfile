FROM debian:trixie-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates iproute2 iputils-ping nftables python3 time procps \
  && rm -rf /var/lib/apt/lists/*

WORKDIR /src
