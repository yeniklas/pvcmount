FROM alpine:3.20
RUN apk add --no-cache openssh-server && ssh-keygen -A
EXPOSE 22
CMD ["/bin/sh", "-c", "mkdir -p /root/.ssh && printf '%s\\n' \"$AUTHORIZED_KEY\" > /root/.ssh/authorized_keys && chmod 700 /root/.ssh && chmod 600 /root/.ssh/authorized_keys && exec /usr/sbin/sshd -D -e -p 22"]
