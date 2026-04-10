#!/bin/sh
set -e

# Start docker daemon
if [ "$1" = "dockerd" ]; then
    # Create certs directory if needed
    if [ -n "$DOCKER_TLS_CERTDIR" ]; then
        mkdir -p "$DOCKER_TLS_CERTDIR"
        # Generate TLS certs if not present
        if [ ! -f "$DOCKER_TLS_CERTDIR/ca.pem" ]; then
            docker daemon --init --host=unix:///var/run/docker.sock --tlsverify --tlscacert="$DOCKER_TLS_CERTDIR/ca.pem" --tlscert="$DOCKER_TLS_CERTDIR/server-cert.pem" --tlskey="$DOCKER_TLS_CERTDIR/server-key.pem" &
            sleep 2
            # Generate certs
            docker --tlsverify --tlscacert="$DOCKER_TLS_CERTDIR/ca.pem" --tlscert="$DOCKER_TLS_CERTDIR/cert.pem" --tlskey="$DOCKER_TLS_CERTDIR/key.pem" \
                run --rm -v "$DOCKER_TLS_CERTDIR:/certs" alpine sh -c "apk add openssl && openssl genrsa -out /certs/ca.key 4096 && openssl req -new -x509 -days 365 -key /certs/ca.key -out /certs/ca.pem -subj '/CN=Docker CA'"
        fi
    fi
    
    exec dockerd --host=unix:///var/run/docker.sock "$@"
fi

exec "$@"
