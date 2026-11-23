FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
COPY migrations ./migrations
RUN CGO_ENABLED=0 go build -trimpath -o /restaurant-ops .
FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /restaurant-ops /restaurant-ops
COPY --from=build /src/migrations ./migrations
EXPOSE 8084
ENTRYPOINT ["/restaurant-ops"]
