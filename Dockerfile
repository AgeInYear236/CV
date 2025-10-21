FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod .
RUN go mod download

COPY main.go .
COPY data ./data
COPY template ./template

RUN CGO_ENABLED=0 GOOS=linux go build -o server main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates

WORKDIR /root/

COPY --from=builder /app/server .
COPY --from=builder /app/data ./data
COPY --from=builder /app/template ./template

EXPOSE 1111

CMD ["./server"]