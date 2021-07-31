##
## Build
##

FROM golang:1.16-buster AS build

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY *.go ./

RUN CGO_ENABLED=0 go build -o /kube-digest-updater

##
## Deploy
##

FROM gcr.io/distroless/static

COPY --from=build /kube-digest-updater /kube-digest-updater

ENTRYPOINT ["/kube-digest-updater"]
