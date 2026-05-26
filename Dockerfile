FROM golang:1.25-alpine AS builder
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w -X github.com/LeeTeng2001/rssify/cmd/version.Version=${VERSION}" -o /rssify .

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=builder /rssify /usr/local/bin/rssify
EXPOSE 8080
ENTRYPOINT ["rssify"]
CMD ["serve"]