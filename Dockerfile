FROM golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /pr-reviewer ./cmd/server

FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=builder /pr-reviewer /app/pr-reviewer

EXPOSE 8080

ENTRYPOINT ["/app/pr-reviewer"]

