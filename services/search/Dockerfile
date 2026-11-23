FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN go mod tidy && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /search-api ./cmd/search-api

FROM alpine:3.20
RUN adduser -D -H app
USER app
COPY --from=build /search-api /search-api
EXPOSE 8083
ENTRYPOINT ["/search-api"]
