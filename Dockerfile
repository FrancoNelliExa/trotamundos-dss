FROM golang:alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY pkg/ ./pkg/
COPY services/ ./services/

ARG SERVICE
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./services/${SERVICE}/cmd

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/server /app/server
COPY api/ ./api/
COPY web/ ./web/

ENTRYPOINT ["/app/server"]
