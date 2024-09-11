FROM golang:1.23-bookworm as build

WORKDIR /go/src/app

COPY go.mod go.sum ./
RUN go mod download

COPY *.go *.go.tpl ./
RUN CGO_ENABLED=0 go build -o /go/bin/app

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /go/bin/app /
CMD ["/app"]
