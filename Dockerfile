FROM golang:1.14 as builder

WORKDIR /go/src/github.com/project0/cronify/

COPY . .
RUN go get -v

WORKDIR /go/src/github.com/project0/cronify/cmd/
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o cronify .

FROM scratch

WORKDIR /root/

COPY --from=builder /etc/ssl/certs /etc/ssl/certs
COPY --from=builder /go/src/github.com/project0/cronify/cmd/cronify .

ENTRYPOINT ["./cronify"]
