FROM golang:1.15 AS builder

# Set some shell options for using pipes and such
SHELL [ "/bin/bash", "-euo", "pipefail", "-c" ]

# Non-privileged containers based on the scratch image
# See https://medium.com/@lizrice/non-privileged-containers-based-on-the-scratch-image-a80105d6d341
RUN useradd -u 10001 scratchuser

# Install/update the common CA certificates package now, and blag it later
RUN apt-get update \
  && apt-get install --assume-yes --no-install-recommends ca-certificates \
  && apt-get autoremove --assume-yes \
  && apt-get clean \
  && rm --force --recursive /root/.cache \
  && rm --force --recursive /var/lib/apt/lists/*

# Don't call any C code (the 'scratch' base image used later won't have any libraries to reference)
ENV CGO_ENABLED=0

# Use Go modules
ENV GO111MODULE=on

# Precompile the entire Go standard library into a Docker cache layer: useful for other projects too!
# See https://www.reddit.com/r/golang/comments/hj4n44/improved_docker_go_module_dependency_cache_for/
RUN go install -ldflags="-buildid= -w" -trimpath -v std

WORKDIR /go/src/go.jlucktay.dev/servers.menagerie.games

# This will save Go dependencies in the Docker cache, until/unless they change
COPY go.mod go.sum ./
RUN go mod download -x

# Download and precompile all third party libraries
RUN go mod graph \
  | awk '$1 !~ "@" { print $2 }' \
  | xargs --no-run-if-empty --verbose \
  go get -ldflags="-buildid= -w" -trimpath -v

# Add the sources
COPY . .

# Compile! Should only compile our project since everything else has been precompiled by now, and future
# (re)compilations will leverage the same cached layer(s)
RUN go build -ldflags="-buildid= -w" -trimpath -o /bin/smg -v go.jlucktay.dev/servers.menagerie.games

FROM scratch AS runner

# Non-privileged containers based on the scratch image
# See https://medium.com/@lizrice/non-privileged-containers-based-on-the-scratch-image-a80105d6d341
COPY --from=builder /etc/passwd /etc/passwd
USER scratchuser

# Bring in web things
WORKDIR /
COPY gsifw.html favicon.ico ./

# Bring common CA certificates and binary over
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /bin/smg /bin/smg

EXPOSE 8080

ENTRYPOINT [ "/bin/smg" ]
