FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fortmont-agent ./cmd/agent

FROM alpine:3.22
RUN addgroup -S fortmont && adduser -S -G fortmont fortmont
COPY --from=build /out/fortmont-agent /usr/local/bin/fortmont-agent
USER fortmont
ENTRYPOINT ["/usr/local/bin/fortmont-agent"]
