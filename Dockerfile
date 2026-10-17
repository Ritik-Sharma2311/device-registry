FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/registry ./cmd/registry

FROM alpine:3.20
COPY --from=build /out/registry /usr/local/bin/registry
EXPOSE 8080
ENTRYPOINT ["registry"]
