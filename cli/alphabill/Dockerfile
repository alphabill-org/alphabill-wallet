FROM golang:1.24-bookworm AS builder

WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN --mount=type=bind,source=ci/.gitconfig,target=/root/.gitconfig \
    --mount=type=bind,source=ci/known_hosts,target=/root/.ssh/known_hosts \
    --mount=type=ssh \
    go mod download && go mod verify

COPY . .
RUN make build

FROM gcr.io/distroless/base-debian12:debug-nonroot
SHELL [ "/busybox/sh", "-c" ]
WORKDIR /app
ENV PATH="/app:${PATH}"
COPY --from=builder /usr/src/app/build/abwallet /app/
ENTRYPOINT ["abwallet"]
