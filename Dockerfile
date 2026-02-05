FROM golang:1.24 as builder
WORKDIR /app
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o query-bench ./query-bench.go

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /app/query-bench .
COPY queries.txt .
EXPOSE 8080
RUN ls -la
RUN pwd
CMD ["./query-bench"]