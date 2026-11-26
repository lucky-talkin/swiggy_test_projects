FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY migrations ./migrations
# Generate the module checksum file from the actual imports before compiling.
RUN go mod tidy
RUN CGO_ENABLED=0 go build -trimpath -o /order-api .
FROM scratch
COPY --from=build /order-api /order-api
COPY --from=build /src/migrations ./migrations
EXPOSE 8086
HEALTHCHECK --interval=30s --timeout=3s CMD ["/order-api", "-healthcheck"]
ENTRYPOINT ["/order-api"]
