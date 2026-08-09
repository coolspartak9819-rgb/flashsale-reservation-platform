FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/flashsale-api ./cmd/api

FROM alpine:3.22
RUN addgroup -S -g 10001 app && adduser -S -D -H -u 10001 -G app app
WORKDIR /app
COPY --from=build /out/flashsale-api /flashsale-api
COPY migrations /app/migrations
USER 10001:10001
EXPOSE 8080
ENTRYPOINT ["/flashsale-api"]
