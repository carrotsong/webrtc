FROM golang:1.14-alpine3.11

RUN apk add --no-cache \
  chromium \
  chromium-chromedriver

ENV CGO_ENABLED=0

COPY . /go/src/github.com/carrotsong/webrtc
WORKDIR /go/src/github.com/carrotsong/webrtc/e2e

CMD ["go", "test", "-tags=e2e", "-v", "."]
