FROM golang:latest
WORKDIR /usr/src/app
COPY go.mod go.sum ./
RUN go mod download && go mod verify
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -o querier cmd/querier/main.go
EXPOSE 8080
CMD ["querier"]