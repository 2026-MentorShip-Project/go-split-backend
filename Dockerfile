FROM golang:1.25-alpine AS build-env

WORKDIR /go-split-backend

RUN apk add build-base git ca-certificates openssh curl

RUN go install github.com/swaggo/swag/cmd/swag@v1.16.6

COPY go.* ./

RUN --mount=type=ssh go mod download

COPY ./cmd ./cmd/
COPY ./internal ./internal/

RUN go generate ./...
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./bin/server.app ./cmd/go-split-backend/server.go


FROM scratch
WORKDIR /srv
COPY --from=build-env /go-split-backend/bin/server.app /srv
EXPOSE 8080
ENTRYPOINT ["/srv/server.app"]
