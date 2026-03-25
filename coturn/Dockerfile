FROM alpine:3.22

RUN apk add --no-cache \
    ca-certificates \
    certbot \
    coturn \
  && update-ca-certificates

COPY entrypoint.sh /entrypoint.sh
COPY certbot-renew.sh /opt/coturn/certbot-renew.sh
COPY on-cert-renew.sh /opt/coturn/on-cert-renew.sh

RUN chmod +x /entrypoint.sh /opt/coturn/certbot-renew.sh /opt/coturn/on-cert-renew.sh \
  && mkdir -p /app

VOLUME ["/app", "/etc/letsencrypt", "/var/lib/letsencrypt"]

ENTRYPOINT ["/entrypoint.sh"]
