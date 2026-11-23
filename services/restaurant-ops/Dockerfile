FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /app .
FROM alpine:3.20
RUN adduser -D app
USER app
COPY --from=build /app /app
EXPOSE 8084
ENTRYPOINT ["/app"]
