FROM golang:1.24-alpine AS build
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o electricity .

FROM alpine:3.19 AS final
WORKDIR /app
COPY --from=build /app/electricity /app/electricity
CMD ["./electricity"]
