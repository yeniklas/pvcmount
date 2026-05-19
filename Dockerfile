FROM alpine:3.20
RUN apk add --no-cache openssh-server && \
    ssh-keygen -A && \
    printf '%s\n' \
      'Port 22' \
      'PermitRootLogin yes' \
      'AuthorizedKeysFile /tmp/authorized_keys' \
      'UsePAM no' \
      'UsePrivilegeSeparation no' \
      'Subsystem sftp /usr/lib/ssh/sftp-server' \
      > /etc/ssh/sshd_config
EXPOSE 22
