FROM golang:1.15 AS builder

# Set some shell options for using pipes and such
SHELL [ "/bin/bash", "-euo", "pipefail", "-c" ]

# Install/update the common CA certificates package now, and blag it later
RUN apt-get update \
  && apt-get install --assume-yes --no-install-recommends ca-certificates \
  && apt-get autoremove --assume-yes \
  && rm --force --recursive /root/.cache

# Don't call any C code (the 'scratch' base image used later won't have any libraries to reference)
ENV CGO_ENABLED=0

# Use Go modules
ENV GO111MODULE=on

# Precompile the entire Go standard library into a Docker cache layer: useful for other projects too!
# cf. https://www.reddit.com/r/golang/comments/hj4n44/improved_docker_go_module_dependency_cache_for/
RUN go install -ldflags="-buildid= -w" -trimpath -v std

WORKDIR /go/src/go.jlucktay.dev/servers.menagerie.games

# This will save Go dependencies in the Docker cache, until/unless they change
COPY go.mod go.sum ./

# Download and precompile all third party libraries
RUN go mod graph \
  | awk '$1 !~ "@" { print $2 }' \
  | xargs --max-args=1 --no-run-if-empty --verbose \
  go get -ldflags="-buildid= -w" -trimpath -v

# Add the sources
COPY . .

# Compile! Should only compile our project since everything else has been precompiled by now, and future
# (re)compilations will leverage the same cached layer(s)
RUN go build -ldflags="-buildid= -w" -trimpath -v -o /bin/smg

FROM scratch AS runner

# Bring common CA certificates and binary over
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /bin/smg /bin/smg

ENTRYPOINT [ "/bin/smg" ]
