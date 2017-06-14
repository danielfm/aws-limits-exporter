FROM centurylink/ca-certs
MAINTAINER Daniel Martins <daniel.martins@descomplica.com.br>

COPY ./bin/aws-limits-exporter /aws-limits-exporter
ENTRYPOINT ["/aws-limits-exporter"]
