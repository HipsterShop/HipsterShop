FROM golang:1.26.1-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /authservice .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /authservice /authservice
EXPOSE 8081
ENTRYPOINT ["/authservice"]
