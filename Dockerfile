FROM golang:1.22-alpine AS build
RUN apk add --no-base git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o electricity .

FROM alpine:3.19 AS final
WORKDIR /app
COPY --from=build /app/electricity /app/electricity
CMD ["./electricity"]
