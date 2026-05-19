FROM alpine:3.20
RUN apk add --no-cache openssh-server && ssh-keygen -A
EXPOSE 22
