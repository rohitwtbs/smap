# Build stage
FROM haskell:9.10-bookworm AS builder

RUN apt-get update && apt-get install -y \
    libpq-dev \
    zlib1g-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build

# Copy only the cabal file first to cache dependencies
COPY hs-archiver.cabal .
RUN cabal update && cabal build --only-dependencies

# Copy the rest of the source code
COPY . .

# Build the executable
RUN cabal build exe:hs-archiver

# Install the executable to a known location
RUN cp $(cabal list-bin exe:hs-archiver) /usr/local/bin/hs-archiver

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    libpq5 \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /usr/local/bin/hs-archiver /usr/local/bin/hs-archiver

EXPOSE 8079

ENTRYPOINT ["hs-archiver"]
