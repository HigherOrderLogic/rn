# This image is used to run builds with pre-downloaded dependencies
# syntax=docker/dockerfile:1
FROM --platform=linux/amd64 golang:1.26-alpine as build

RUN apk add --no-cache make git openssh gcc g++ musl-dev linux-headers

# for ebiten
RUN apk add alsa-lib-dev libx11-dev libxrandr-dev libxcursor-dev libxinerama-dev \
	libxi-dev mesa-utils mesa-dev pkgconf

ARG GIT_SSH_KEY
WORKDIR /src
COPY . .

RUN git config --global url.ssh://git@github.com/.insteadOf https://github.com/
RUN mkdir -p ~/.ssh && chmod 700 ~/.ssh && \
	printf '%s\n' "$GIT_SSH_KEY" | tr -d '\r' > ~/.ssh/id_rsa && \
	chmod 600 ~/.ssh/id_rsa
RUN ssh-keyscan -t rsa git.unstable.build >> ~/.ssh/known_hosts
RUN ssh-keyscan -t rsa github.com >> ~/.ssh/known_hosts
RUN go env -w GOPRIVATE="github.com/unstablebuild,unstable.build/*"
RUN go env -w GOCACHE="/go/cache"

# forces to download and compile all dependencies
RUN make debug && \
	go test -c -o /dev/null $(go list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' ./...) && \
	cp bin/* /go/bin
RUN go install github.com/unstablebuild/blue/cmd/bluectl@latest
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.60.3

# use clean image with no source code! 
FROM --platform=linux/amd64 golang:1.26-alpine
COPY --from=build /go /go
RUN apk add --no-cache make git openssh gcc g++ musl-dev linux-headers bash
RUN apk add --no-cache glfw-dev alsa-lib-dev libx11-dev libxrandr-dev \
	libxcursor-dev libxinerama-dev libxi-dev mesa-utils mesa-dev pkgconf \
	xvfb-run
RUN rm -rf /go/pkg/mod/unstable.build
