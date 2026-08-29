FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /sahayak-api ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /sahayak-api /sahayak-api
EXPOSE 8080
ENTRYPOINT ["/sahayak-api"]
