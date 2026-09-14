FROM golang:1.25.6 AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/control-api ./cmd/control-api
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/control-api /control-api
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/control-api"]
