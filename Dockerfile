FROM alpine:latest
ARG TARGETARCH
RUN apk --no-cache add ca-certificates
WORKDIR /app/
ADD .build/pier_linux_${TARGETARCH}.tar.gz .
EXPOSE 53/udp 80 443
ENTRYPOINT ["./pier"]
