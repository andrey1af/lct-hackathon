#!/bin/bash
set -e

if ! command -v docker >/dev/null 2>&1; then
    echo "Error: docker is not installed." >&2
    exit 1
fi

domains=(vozduhanchiki.ru www.vozduhanchiki.ru)
rsa_key_size=4096
data_path="./deploy/certbot"
email="${CERTBOT_EMAIL:-}"
staging="${CERTBOT_STAGING:-0}"

if [ -d "$data_path/conf/live/${domains[0]}" ]; then
    read -p "Existing certificate data found for ${domains[0]}. Continue and replace it? (y/N) " decision
    if [ "$decision" != "Y" ] && [ "$decision" != "y" ]; then
        exit 0
    fi
fi

certbot_ref="v5.8.0"
certbot_raw="https://raw.githubusercontent.com/certbot/certbot/$certbot_ref"

if [ ! -e "$data_path/conf/options-ssl-nginx.conf" ] || [ ! -e "$data_path/conf/ssl-dhparam.pem" ]; then
    echo "### Downloading recommended TLS parameters ..."
    mkdir -p "$data_path/conf"
    curl -fsS "$certbot_raw/certbot/src/certbot/_internal/plugins/nginx/tls_configs/options-ssl-nginx.conf" -o "$data_path/conf/options-ssl-nginx.conf"
    curl -fsS "$certbot_raw/certbot/src/certbot/ssl-dhparams.pem" -o "$data_path/conf/ssl-dhparam.pem"
fi

echo "### Creating dummy certificate for ${domains[0]} ..."
path="/etc/letsencrypt/live/${domains[0]}"
mkdir -p "$data_path/conf/live/${domains[0]}"
docker compose run --rm --entrypoint "\
  openssl req -x509 -nodes -newkey rsa:$rsa_key_size -days 1 \
    -keyout '$path/privkey.pem' \
    -out '$path/fullchain.pem' \
    -subj '/CN=localhost'" certbot

echo "### Starting proxy ..."
docker compose up --force-recreate -d proxy

echo "### Waiting for proxy to accept connections on port 80 ..."
ready=0
for i in $(seq 1 15); do
    code="$(curl -s -o /dev/null -w '%{http_code}' http://localhost/ 2>/dev/null || true)"
    if [ -n "$code" ] && [ "$code" != "000" ]; then
        ready=1
        break
    fi
    sleep 1
done
if [ "$ready" -ne 1 ]; then
    echo "Error: proxy did not respond on port 80. Check 'docker compose logs proxy' for nginx errors before requesting a certificate." >&2
    exit 1
fi

echo "### Deleting dummy certificate for ${domains[0]} ..."
docker compose run --rm --entrypoint "\
  rm -Rf /etc/letsencrypt/live/${domains[0]} && \
  rm -Rf /etc/letsencrypt/archive/${domains[0]} && \
  rm -Rf /etc/letsencrypt/renewal/${domains[0]}.conf" certbot

echo "### Requesting Let's Encrypt certificate for ${domains[*]} ..."
domain_args=""
for domain in "${domains[@]}"; do
    domain_args="$domain_args -d $domain"
done

if [ -z "$email" ]; then
    email_arg="--register-unsafely-without-email"
else
    email_arg="--email $email"
fi

staging_arg=""
if [ "$staging" != "0" ]; then
    staging_arg="--staging"
fi

docker compose run --rm --entrypoint "\
  certbot certonly --webroot -w /var/www/certbot \
    $staging_arg \
    $email_arg \
    $domain_args \
    --rsa-key-size $rsa_key_size \
    --agree-tos \
    --force-renewal" certbot

echo "### Reloading proxy ..."
docker compose exec proxy nginx -s reload

echo "Done. https://${domains[0]} should now serve a valid certificate."
