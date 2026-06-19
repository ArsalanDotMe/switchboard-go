FROM golang:1.22-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/switchboard-go ./...

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/switchboard-go /usr/local/bin/switchboard-go
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/switchboard-go"]
