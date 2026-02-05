ARG TARGETOS
ARG TARGETARCH
ARG BUILDPLATFORM
FROM --platform=${BUILDPLATFORM} golang:1.24 AS builder
WORKDIR /app
COPY go.mod ./
COPY . .
# Use the target platform architecture provided by buildx (TARGETOS/TARGETARCH)
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} \
	go build -ldflags='-s -w' -o query-bench ./query-bench.go

FROM --platform=${TARGETOS}/${TARGETARCH} alpine:3.21
WORKDIR /app
COPY --from=builder /app/query-bench .
COPY queries.txt .
EXPOSE 8080
CMD ["./query-bench"]