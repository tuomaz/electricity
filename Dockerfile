FROM golang:1.20-alpine AS build
RUN apk add git
RUN mkdir /build
WORKDIR /build
RUN git clone https://github.com/tuomaz/electricity.git
WORKDIR /build/electricity
RUN go build

FROM alpine:3.17 AS final
COPY --from=build /build/electricity/electricity electricity
CMD ./electricity