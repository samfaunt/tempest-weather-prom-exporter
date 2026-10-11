FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /tempest-exporter .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /tempest-exporter /tempest-exporter
EXPOSE 7070 50222/udp
ENTRYPOINT ["/tempest-exporter"]
