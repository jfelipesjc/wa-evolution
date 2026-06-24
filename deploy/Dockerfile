# Build a partir do diretório-PAI que contém ./wa-go e ./wa-evolution
# (o go.mod usa replace github.com/felipeleal/wa-go => ../wa-go).
#   docker build -f wa-evolution/deploy/Dockerfile -t wa-evolution .
FROM golang:1.22-alpine AS build
WORKDIR /src
COPY wa-go/ ./wa-go/
COPY wa-evolution/ ./wa-evolution/
WORKDIR /src/wa-evolution
RUN CGO_ENABLED=0 go build -ldflags='-s -w' -o /out/wa-server ./cmd/wa-server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 wa
WORKDIR /opt/wa-evolution
COPY --from=build /out/wa-server /opt/wa-evolution/wa-server
USER wa
EXPOSE 8091
VOLUME ["/opt/wa-evolution/instances"]
ENTRYPOINT ["/opt/wa-evolution/wa-server"]
CMD ["-addr", ":8091", "-dir", "/opt/wa-evolution/instances"]
