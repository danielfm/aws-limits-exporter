# build stage
FROM golang:1.12-alpine AS build-env
RUN apk add --no-cache --update alpine-sdk
ENV REPO_PATH=/build
COPY . $REPO_PATH
WORKDIR $REPO_PATH
RUN make build

# final stage
FROM alpine
LABEL maintainer="Daniel Martins <daniel.martins@descomplica.com.br>"
WORKDIR /app
COPY --from=build-env /build/bin/aws-limits-exporter /app/
RUN apk add --no-cache --update ca-certificates
ENTRYPOINT /app/aws-limits-exporter -logtostderr
