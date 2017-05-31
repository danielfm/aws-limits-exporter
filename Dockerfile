FROM centurylink/ca-certs
MAINTAINER Daniel Martins <daniel.martins@descomplica.com.br>

COPY ./bin/kube-aws-limits-monitor /kube-aws-limits-monitor
ENTRYPOINT ["/kube-aws-limits-monitor"]
