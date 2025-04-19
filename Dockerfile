FROM --platform=$BUILDPLATFORM golang:1-alpine AS build

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

ENV CGO_ENABLED=0
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=$TARGETOS
ENV GOARCH=$TARGETARCH
RUN go build -o app

FROM alpine:latest

WORKDIR /app

COPY --from=build /app ./

ENTRYPOINT ["/app/app"]
