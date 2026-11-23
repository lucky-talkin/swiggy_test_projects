FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY *.go ./
COPY migrations ./migrations
RUN CGO_ENABLED=0 go build -trimpath -o /order-api .
FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /order-api /order-api
COPY --from=build /src/migrations ./migrations
EXPOSE 8086
ENTRYPOINT ["/order-api"]
