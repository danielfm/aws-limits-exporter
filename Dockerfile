# build stage
FROM golang:1.22-alpine AS build-env
RUN apk add --no-cache --update ca-certificates
ENV REPO_PATH=/build
COPY . $REPO_PATH
WORKDIR $REPO_PATH
RUN make build

# final stage
FROM alpine:3.21.3
LABEL maintainer="Subhash Kanchi <skanchi@pm.me>" version="$VERSION"
RUN apk upgrade --no-cache 
WORKDIR /app
COPY --from=build-env /build/bin/aws-limits-exporter /app/
RUN apk add --no-cache --update ca-certificates
ENTRYPOINT /app/aws-limits-exporter -logtostderr
